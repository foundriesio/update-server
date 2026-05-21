"""pytest fixtures for dg-satellite + fioup e2e tests."""

import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path

import paramiko
import pytest
import requests

REPO_ROOT = Path(__file__).parent
CACHE_DIR = REPO_ROOT / ".cache"

TOOLS_IMAGE = "dg-sat-e2e-tools"

DG_SAT_URL = (
    "https://github.com/foundriesio/dg-satellite/releases/download/v0.7/"
    "dg-sat-linux-amd64"
)
FIOUP_DEB_URL = (
    "https://github.com/foundriesio/fioup/releases/download/v1.3.3/"
    "fioup_1.3.3_amd64.deb"
)
# genericcloud variant includes cloud-init
DEBIAN_IMAGE_URL = (
    "https://cloud.debian.org/images/cloud/trixie/latest/"
    "debian-13-genericcloud-amd64.qcow2"
)

SOTA_TOML = """\
[import]
tls_cacert_path = "/var/sota/root.crt"
tls_clientcert_path = "/var/sota/client.pem"
tls_pkey_path = "/var/sota/pkey.pem"

[uptane]
key_source = "file"
polling_sec = 30
repo_server = "https://dg-satellite:8443/repo"

[provision]
primary_ecu_hardware_id = "intel-corei7-64"
server = "https://dg-satellite:8443"

[pacman]
compose_apps_root = "/var/sota/compose-apps"
compose_apps_proxy = "https://dg-satellite:8443/app-proxy-url"
ostree_server = "https://dg-satellite:8443/ostree"
packages_file = "/usr/package.manifest"
reset_apps = " "
reset_apps_root = "/var/sota/reset-apps"
tags = "main"
type = "ostree+compose_apps"

[storage]
path = "/var/sota/"
type = "sqlite"

[tls]
server = "https://dg-satellite:8443"
ca_source = "file"
cert_source = "file"
pkey_source = "file"
"""

VM_SSH_PORT = 2222
SERVER_UI_PORT = 8080


def _check_tools():
    missing = []
    for tool in ("docker", "openssl"):
        if not shutil.which(tool):
            missing.append(tool)
    if missing:
        sys.exit("Missing required host tools: " + ", ".join(missing))


def _download(url: str, dest: Path):
    if dest.exists():
        return
    print(f"\n[setup] Downloading {dest.name} ...", flush=True)
    CACHE_DIR.mkdir(exist_ok=True)
    tmp = dest.with_suffix(".tmp")
    try:
        urllib.request.urlretrieve(url, tmp)
        tmp.rename(dest)
    except Exception:
        tmp.unlink(missing_ok=True)
        raise
    print(f"[setup] Downloaded {dest.name}", flush=True)


def _build_tools_image():
    result = subprocess.run(
        ["docker", "image", "inspect", TOOLS_IMAGE],
        capture_output=True,
    )
    if result.returncode == 0:
        return
    print(f"\n[setup] Building {TOOLS_IMAGE} Docker image ...", flush=True)
    subprocess.run(
        ["docker", "build", "-t", TOOLS_IMAGE, str(REPO_ROOT)],
        check=True,
    )
    print(f"[setup] {TOOLS_IMAGE} image ready", flush=True)


def _docker_run(cmd: list[str], volumes: dict[str, str] | None = None,
                extra_args: list[str] | None = None) -> subprocess.CompletedProcess:
    docker_cmd = ["docker", "run", "--rm"]
    for host_path, container_path in (volumes or {}).items():
        docker_cmd += ["-v", f"{host_path}:{container_path}"]
    if extra_args:
        docker_cmd += extra_args
    docker_cmd += [TOOLS_IMAGE] + cmd
    return subprocess.run(docker_cmd, check=True, capture_output=True, text=True)


def _make_seed_iso(user_data: str, meta_data: str, out_path: Path):
    """Create a cloud-init nocloud seed ISO using genisoimage in Docker."""
    out_dir = out_path.parent
    (out_dir / "user-data").write_text(user_data)
    (out_dir / "meta-data").write_text(meta_data)
    network_config = (REPO_ROOT / "cloud-init" / "network-config").read_text()
    (out_dir / "network-config").write_text(network_config)
    _docker_run(
        ["genisoimage", "-o", "/workdir/seed.iso", "-V", "cidata",
         "-r", "-J", "/workdir/user-data", "/workdir/meta-data",
         "/workdir/network-config"],
        volumes={str(out_dir): "/workdir"},
    )


def _wait_tcp(host: str, port: int, timeout: int = 300):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection((host, port), timeout=3):
                return
        except OSError:
            time.sleep(2)
    raise TimeoutError(f"Port {port} on {host} did not open within {timeout}s")


class SshClient:
    """Thin paramiko wrapper for running commands in the VM."""

    def __init__(self, host: str, port: int, key_path: Path):
        self._host = host
        self._port = port
        self._key_path = key_path
        self._client: paramiko.SSHClient | None = None

    def connect(self, timeout: int = 180):
        """Wait up to timeout seconds for SSH to become available."""
        _wait_tcp(self._host, self._port, timeout=timeout)
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        key = paramiko.RSAKey.from_private_key_file(str(self._key_path))
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                client.connect(
                    self._host,
                    port=self._port,
                    username="root",
                    pkey=key,
                    timeout=10,
                    banner_timeout=30,
                    auth_timeout=30,
                    look_for_keys=False,
                    allow_agent=False,
                )
                self._client = client
                return
            except (paramiko.ssh_exception.NoValidConnectionsError,
                    paramiko.ssh_exception.SSHException,
                    OSError):
                time.sleep(3)
        raise TimeoutError("SSH did not become available within timeout")

    def run(self, cmd: str, check: bool = True) -> tuple[str, str]:
        assert self._client, "Not connected"
        _, stdout, stderr = self._client.exec_command(cmd, timeout=45)
        out = stdout.read().decode()
        err = stderr.read().decode()
        rc = stdout.channel.recv_exit_status()
        if check and rc != 0:
            raise RuntimeError(
                f"Command failed (rc={rc}): {cmd!r}\nstdout={out}\nstderr={err}"
            )
        return out, err

    def put(self, local: Path, remote: str):
        assert self._client, "Not connected"
        sftp = self._client.open_sftp()
        sftp.put(str(local), remote)
        sftp.close()

    def put_text(self, content: str, remote: str):
        assert self._client, "Not connected"
        sftp = self._client.open_sftp()
        with sftp.file(remote, "w") as f:
            f.write(content)
        sftp.close()

    def close(self):
        if self._client:
            self._client.close()
            self._client = None


@pytest.fixture(scope="session", autouse=True)
def preflight():
    _check_tools()
    _build_tools_image()


@pytest.fixture(scope="session")
def dg_sat_bin(preflight) -> Path:
    dest = CACHE_DIR / "dg-sat"
    _download(DG_SAT_URL, dest)
    dest.chmod(0o755)
    return dest


@pytest.fixture(scope="session")
def fioup_deb(preflight) -> Path:
    dest = CACHE_DIR / "fioup.deb"
    _download(FIOUP_DEB_URL, dest)
    return dest


@pytest.fixture(scope="session")
def debian_image(preflight) -> Path:
    dest = CACHE_DIR / "debian-trixie.qcow2"
    _download(DEBIAN_IMAGE_URL, dest)
    return dest


@pytest.fixture(scope="session")
def dg_satellite_server(request, dg_sat_bin):
    """Generate PKI, start dg-satellite; yield datadir Path."""
    datadir = Path(tempfile.mkdtemp(prefix="dg-sat-"))
    gen_pki = REPO_ROOT / "scripts" / "gen_pki.sh"

    print("\n[setup] Generating PKI ...", flush=True)
    result = subprocess.run(
        ["bash", str(gen_pki), str(datadir), str(dg_sat_bin),
         "dg-satellite", "e2e-factory"],
        check=True, capture_output=True, text=True,
    )
    print(result.stdout)

    print("[setup] Initialising auth (noauth/test mode) ...", flush=True)
    subprocess.run(
        [str(dg_sat_bin), "--datadir", str(datadir), "auth-init", "--test"],
        check=True, capture_output=True,
    )

    print("[setup] Starting dg-satellite server ...", flush=True)
    log_path = datadir / "server.log"
    log_file = open(log_path, "w")
    proc = subprocess.Popen(
        [str(dg_sat_bin), "serve", "--datadir", str(datadir)],
        stdout=log_file, stderr=log_file,
    )

    deadline = time.time() + 30
    while time.time() < deadline:
        try:
            requests.get(f"http://localhost:{SERVER_UI_PORT}", timeout=2)
            break
        except requests.exceptions.ConnectionError:
            time.sleep(1)
    else:
        proc.kill()
        log_file.close()
        print(log_path.read_text())
        raise RuntimeError("dg-satellite did not start within 30s")

    print(f"[setup] dg-satellite running (pid={proc.pid})", flush=True)

    yield datadir

    proc.terminate()
    proc.wait(timeout=10)
    log_file.close()
    if request.session.testsfailed:
        print("\n[teardown] dg-satellite log:\n" + log_path.read_text(), flush=True)
    shutil.rmtree(datadir, ignore_errors=True)


@pytest.fixture(scope="session")
def fioup_vm(preflight, dg_satellite_server, fioup_deb, debian_image):
    """Boot a Debian trixie QEMU VM (via Docker) with fioup installed.

    The genericcloud image has cloud-init pre-installed; the CIDATA seed ISO
    configures root SSH access and the /etc/hosts dg-satellite alias.
    After SSH is available, fioup is installed and device credentials are copied.
    """
    workdir = Path(tempfile.mkdtemp(prefix="fioup-vm-"))
    datadir = dg_satellite_server

    # Ephemeral SSH keypair
    key = paramiko.RSAKey.generate(bits=2048)
    key_path = workdir / "test_key"
    key.write_private_key_file(str(key_path))
    pubkey = f"ssh-rsa {key.get_base64()} fioup-e2e"

    # Overlay disk image backed by the cached genericcloud base
    _docker_run(
        ["qemu-img", "create",
         "-b", "/cache/debian-trixie.qcow2",
         "-F", "qcow2", "-f", "qcow2",
         "/workdir/vm.qcow2", "10G"],
        volumes={str(workdir): "/workdir", str(CACHE_DIR): "/cache"},
    )

    # Cloud-init CIDATA seed: root SSH key + /etc/hosts alias for dg-satellite
    template = (REPO_ROOT / "cloud-init" / "user-data.template").read_text()
    user_data = template.format(ssh_public_key=pubkey)
    meta_data = (REPO_ROOT / "cloud-init" / "meta-data").read_text()
    _make_seed_iso(user_data, meta_data, workdir / "seed.iso")

    # Start QEMU in a detached Docker container
    container_name = f"qemu-e2e-{int(time.time())}"
    qemu_args = [
        "qemu-system-x86_64",
        "-m", "1024", "-smp", "2",
        "-nographic",
        "-hda", "/workdir/vm.qcow2",
        "-cdrom", "/workdir/seed.iso",
        "-netdev", f"user,id=net0,hostfwd=tcp::{VM_SSH_PORT}-:22",
        "-device", "virtio-net-pci,netdev=net0",
    ]
    if Path("/dev/kvm").exists():
        qemu_args += ["-cpu", "host", "-enable-kvm"]

    docker_cmd = [
        "docker", "run", "-d",
        "--name", container_name,
        "--network=host",
        "-v", f"{workdir}:/workdir",
        "-v", f"{CACHE_DIR}:/cache",
    ]
    if Path("/dev/kvm").exists():
        docker_cmd += ["--device", "/dev/kvm"]
    docker_cmd += [TOOLS_IMAGE] + qemu_args

    print("\n[setup] Starting QEMU VM in Docker ...", flush=True)
    subprocess.run(docker_cmd, check=True, capture_output=True)

    ssh = SshClient("127.0.0.1", VM_SSH_PORT, key_path)
    try:
        print("[setup] Waiting for SSH (up to 3 min) ...", flush=True)
        ssh.connect(timeout=180)
        print("[setup] SSH ready; installing fioup ...(also triggers docker, etc)", flush=True)

        ssh.put(fioup_deb, "/tmp/fioup.deb")
        ssh.run("DEBIAN_FRONTEND=noninteractive apt update")
        ssh.run("DEBIAN_FRONTEND=noninteractive apt install -y /tmp/fioup.deb")

        ssh.run("/usr/bin/fioup version")
        print("[setup] Copying device credentials ...", flush=True)
        ssh.run("mkdir -p /var/sota")
        device_dir = datadir / "device"
        ssh.put(device_dir / "root.crt", "/var/sota/root.crt")
        ssh.put(device_dir / "client.pem", "/var/sota/client.pem")
        ssh.put(device_dir / "pkey.pem", "/var/sota/pkey.pem")
        ssh.put_text(SOTA_TOML, "/var/sota/sota.toml")

        print("[setup] VM ready", flush=True)
        yield ssh

    finally:
        ssh.close()
        subprocess.run(["docker", "stop", container_name], capture_output=True)
        subprocess.run(["docker", "rm", "-f", container_name], capture_output=True)
        shutil.rmtree(workdir, ignore_errors=True)

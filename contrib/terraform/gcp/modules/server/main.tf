# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# The instance, its persistent data disk, the secret escrow, and backups.

terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

locals {
  labels = merge(var.labels, { name = var.name_prefix })

  # Secret Manager IDs cannot contain "/" or ".", unlike Secrets Manager's
  # path-style names, so this collapses the hostname into dashes.
  secret_prefix = "${var.name_prefix}-${replace(var.hostname, ".", "-")}"

  # Written by scripts/init-secrets.sh, never by Terraform or the instance.
  # Declared here so the IAM bindings can name them explicitly rather than
  # using a project-wide role grant.
  escrowed_secrets = ["hmac-secret", "certs-archive", "tuf-archive"]

  # The load balancer connects over the network, so loopback is insufficient.
  ui_addr = "0.0.0.0:8080"

  # Configuration only -- never secrets. The instance fetches those from
  # Secret Manager using its service account token, so nothing sensitive
  # lands in user_data (which is readable via the metadata server) or in
  # Terraform state.
  user_data = <<-EOT
    #cloud-config
    write_files:
      - path: /etc/fioserver/env
        permissions: '0644'
        content: |
          FIOSERVER_HOSTNAME=${var.hostname}
          FIOSERVER_SECRET_PREFIX=${local.secret_prefix}
          FIOSERVER_UI_ADDR=${local.ui_addr}
          FIOSERVER_GATEWAY_ADDR=0.0.0.0:${var.gateway_port}
%{if var.enable_cloud_logging~}
      - path: /etc/google-cloud-ops-agent/config.yaml
        permissions: '0644'
        content: |
          logging:
            receivers:
              fioserver_journald:
                type: systemd_journald
            service:
              pipelines:
                fioserver:
                  receivers: [fioserver_journald]
%{endif~}
    runcmd:
      - [systemctl, enable, --now, fioserver-volume-init.service]
      - [systemctl, enable, --now, data.mount]
      - [systemctl, enable, --now, fioserver-bootstrap.service]
      - [systemctl, enable, --now, fioserver.service]
%{if var.enable_cloud_logging~}
      - [systemctl, restart, google-cloud-ops-agent]
%{endif~}
  EOT
}

# ------------------------------------------------------------- data disk ----
# A standalone disk, deliberately not the boot disk, so the server's state
# survives replacing the instance. There is no prevent_destroy: it would make
# `terraform destroy` fail and strand the disk. The snapshot schedule and the
# secret escrow are the recovery path -- see the README.
resource "google_compute_disk" "data" {
  name = "${var.name_prefix}-data"
  zone = var.zone
  type = "pd-balanced"
  size = var.data_volume_size

  labels = merge(local.labels, {
    # The snapshot policy is attached to this disk directly (see below), so
    # unlike the AWS DLM policy there is no tag-based selector to maintain.
    name = "${var.name_prefix}-data"
  })
}

# ---------------------------------------------------------------- secrets ----
# Terraform never creates these -- scripts/init-secrets.sh does, before
# `apply` runs. Looking them up with a data source (rather than owning them
# as a resource) means a deployment that skips the script fails fast here,
# at plan/apply time, instead of succeeding and leaving the instance to fail
# loudly at boot.
data "google_secret_manager_secret" "auth_config" {
  secret_id = "${local.secret_prefix}-auth-config"
}

data "google_secret_manager_secret" "escrow" {
  for_each = toset(local.escrowed_secrets)

  secret_id = "${local.secret_prefix}-${each.key}"
}

# ------------------------------------------------------------------- IAM ----
resource "google_service_account" "server" {
  # account_id must be <= 30 chars, lowercase alphanumeric/hyphens, and
  # unique within the project -- a hash of the hostname keeps it short and
  # collision-free across deployments that share a name_prefix.
  account_id   = "${var.name_prefix}-${substr(md5(var.hostname), 0, 8)}"
  display_name = "${var.name_prefix} update server (${var.hostname})"
}

# Scoped to exactly the four secrets this deployment owns, not a project-wide
# role. Read-only: the instance only ever restores from escrow (see
# fioserver-bootstrap.sh's State B); escrow is populated by init-secrets.sh.
resource "google_secret_manager_secret_iam_member" "auth_config" {
  secret_id = data.google_secret_manager_secret.auth_config.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.server.email}"
}

resource "google_secret_manager_secret_iam_member" "escrow" {
  for_each = data.google_secret_manager_secret.escrow

  secret_id = each.value.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.server.email}"
}

resource "google_project_iam_member" "cloud_logging" {
  for_each = var.enable_cloud_logging ? toset(["roles/monitoring.metricWriter", "roles/logging.logWriter"]) : []

  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${google_service_account.server.email}"
}

# --------------------------------------------------------------- backups ----
resource "google_compute_resource_policy" "data_snapshot" {
  name   = "${var.name_prefix}-data-snapshot"
  region = var.region

  snapshot_schedule_policy {
    schedule {
      daily_schedule {
        days_in_cycle = 1
        start_time    = var.snapshot_start_time
      }
    }

    retention_policy {
      max_retention_days    = var.snapshot_retention_days
      on_source_disk_delete = "KEEP_AUTO_SNAPSHOTS"
    }

    # Note: these snapshots are crash-consistent. SQLite in WAL mode recovers
    # from them in practice, but see the README for the caveat.
  }
}

resource "google_compute_disk_resource_policy_attachment" "data" {
  name = google_compute_resource_policy.data_snapshot.name
  disk = google_compute_disk.data.name
  zone = var.zone
}

# ---------------------------------------------------------------- address ----
resource "google_compute_address" "server" {
  count = var.assign_static_ip ? 1 : 0

  name   = "${var.name_prefix}-server"
  region = var.region
}

# --------------------------------------------------------------- instance ----
resource "google_compute_instance" "server" {
  name         = "${var.name_prefix}-server"
  zone         = var.zone
  machine_type = var.machine_type
  tags         = [var.server_tag]
  labels       = local.labels

  boot_disk {
    initialize_params {
      image = var.image
      size  = var.root_volume_size
      type  = "pd-balanced"
    }
  }

  # Attached with an explicit device_name so its path is deterministic
  # (/dev/disk/by-id/google-fiodata) -- unlike the AWS module, there is no
  # need for fioserver-volume-init to search for "the blank disk".
  attached_disk {
    source      = google_compute_disk.data.self_link
    device_name = "fiodata"
  }

  network_interface {
    subnetwork = var.subnetwork_self_link

    # Always gets an external IP, static or ephemeral. Unlike the AWS
    # load-balancer topology (whose instance gets neither a public IP nor a
    # NAT Gateway, leaving it with no internet egress), reachability here is
    # controlled entirely by the firewall rules in modules/network, not by
    # withholding a public IP. See the README for the full rationale.
    access_config {
      nat_ip = var.assign_static_ip ? google_compute_address.server[0].address : null
    }
  }

  service_account {
    email = google_service_account.server.email
    # Fine-grained access comes from the IAM bindings above, not from a
    # narrow scope list -- cloud-platform is required for the metadata-server
    # token the instance uses to call the Secret Manager REST API.
    scopes = ["cloud-platform"]
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  metadata = merge(
    {
      "enable-oslogin" = "TRUE"
      "user-data"      = local.user_data
    },
    var.ssh_public_key == "" ? {} : { "ssh-keys" = "admin:${var.ssh_public_key}" },
  )

  lifecycle {
    # Rebuilding the image is the OS patching path; adopt a new one only when
    # the operator explicitly replaces the instance.
    ignore_changes = [boot_disk[0].initialize_params[0].image]
  }
}

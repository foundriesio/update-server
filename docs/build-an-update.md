# How to Build an Update

Producing an update for this server is a three-step process:

1. Build a platform (OS) image — this produces an `ostree_repo`.
2. Build your containers and compose app(s).
3. Combine the `ostree_repo` and your app(s) into an offline-update
   directory, ready to be uploaded per the [updates](./updates.md) guide.

## Platform Build

### Using an Existing FoundriesFactory

> See [Building From Source](https://docs.foundries.io/97/user-guide/lmp-customization/linux-building.html)
> for the full walkthrough. This section only covers the parts specific to
> producing update content for this server.

```
  repo init -u https://source.foundries.io/factories/<factory-name>/lmp-manifest.git -b main -m <factory-name>.xml
  repo sync
  MACHINE=<machine-name> source setup-environment [BUILDDIR]
```

Before building, set `H_BUILD` in `conf/local.conf` to a number that
identifies this build:

```
  echo 'H_BUILD = "148"' >> conf/local.conf
```

> [!NOTE]
> `H_BUILD` is just a build number for the platform image — pick
> any incrementing value you like. It does not need to match or correlate
> with your app build numbers; the platform and app builds are versioned
> independently and only get tied together in the [Combine](#combine) step.
> The first time you build, its recommended to set H_BUILD to your latest
> FoundriesFactory target number and add 1.

Next, build lmp-device-register to point at this update server:
```
echo LMP_DEVICE_API = "https://<YOUR SERVER>/v1/devices" >> conf/local.conf
echo LMP_OAUTH_API = "https://<YOUR SERVER>/oauth2" >> conf/local.conf
```

Now build:

```
  bitbake lmp-factory-image
```

Once the build finishes, your `ostree_repo` is under:

```
  deploy/images/<machine-name>/ostree_repo
```

That's the directory referenced as `ostree_repo` throughout the
[updates](./updates.md) guide.

### Without FoundriesFactory (meta-foundries + QLI)

In this flow the registration tool is meta-foundries' `fio-device-register`,
configured with `FIO_DEVICE_API` the same way.

<!-- TODO: document building the platform without a FoundriesFactory.-->

## Build Apps

### Build and Push Container Images

Build each container image, push it to a registry, and export it into the
update directory as an OCI image layout. Capture the pushed image's digest
so it can be pinned into the compose app:

```
  docker buildx build --platform linux/amd64,linux/arm64 \
    -t <registry>/<image-name>:<tag> \
    --output type=registry \
    --output type=oci,dest=./148/apps,tar=false .
```

Set `--platform` to the target device platforms. This example builds for
both AMD64 and ARM64; use `--platform linux/arm64` for ARM64 only. Ensure
your [builder supports the target platforms](https://docs.docker.com/build/building/multi-platform/).

The OCI export saves container layers in `./148/apps` for reuse by
`composectl pull`. `tar=false` writes a directory layout; the default is a
tar archive. [Multiple outputs](https://docs.docker.com/build/exporters/#multiple-exporters)
require Buildx and BuildKit 0.13.0 or later. The selected builder must
support [OCI export](https://docs.docker.com/build/exporters/oci-docker/),
for example through the `docker-container` driver.

The push output (or `docker/build-push-action`'s `digest` output, if
you're doing this via CI) gives you a `sha256` for the image — you will pin
that in the next step rather than trusting a mutable tag.

### Build and Publish the Compose App

Write a `docker-compose.yml` referencing your image(s). Publishing it
with `composectl publish` produces the sha256 for the app itself:

```
  composectl publish -d app.hash \
    [--pinned-images <registry>/<image-name>@sha256:<image-digest>] \
    <registry>/<app-name>-app:<tag> amd64,arm64
```

* `-d app.hash` writes the app's own digest to the file `app.hash`.
* `--pinned-images` optional, takes a comma-separated list of image digest URIs and
  rewrites `docker-compose.yml`'s image references to match, so you do not
  need to hand-edit digests into the compose file yourself. Omit it if
  your compose file already tags or pins images directly.
* The trailing argument is the comma-separated list of architectures to
  publish for.

The app's own sha256 is now in `app.hash`, giving you an app URI of
`<registry>/<app-name>-app@sha256:<contents of app.hash>`.

You can also refer to the [example GitHub Workflow](./gh-workflow-example.yml).

### Get the App to the Update Server

Complete the update directory by pulling the published app:

```
  composectl pull --arch arm64 -i ./148/apps -s ./148/apps \
    <registry>/<app-name>-app@sha256:<contents of app.hash>
```

Use the same `./148/apps` directory as the OCI export above.
`composectl pull` reuses the exported container layers and downloads only
missing app content.

Set `--arch` to the target device architecture, regardless of the host
architecture. It must be included in both the container build platforms
and the architectures passed to `composectl publish`.

This produces the `apps/apps/<app-name>/<sha256>/` layout.
`fiocli updates upload` uploads that content and automatically discovers
the app name and digest.

You can override the discovered app metadata with
`--apps <name>=<sha256>`. This flag does not download app content or
replace the requirement to include it in the upload directory.

> [!NOTE]
> The update server does not need access to your registry because the
> app and container content is downloaded before upload and included
> in the update directory.

## Combine

With an `ostree_repo` from the platform build and one or more app
name/sha256 pairs from the app build run:
```
  fiocli updates upload main 148 ./148  --version 148 \
```

> [!NOTE] if your apps change between updates that share the same
> ostree build, do not rely on the probed `version` (from `IMAGE_VERSION`,
> which comes from your platform's `H_BUILD`) — it will collide. Pass a
> distinct `--version` for each update instead, as described in
> [updates.md](./updates.md#automatic-tuf-target-generation).

One common approach to creating an app version:
 * Take the `H_BUILD` value (`ostree --repo=148/ostree_repo repo  <ref> cat /etc/os-release`) and multiply by 1000
 * Add your apps build number.

For this example with platform build 148 and say, apps version 42,
the version could become 148042.

Once combined, follow the rest of the [updates](./updates.md) guide to
upload and roll out the result.

# How to build an Update

Producing an update for this server is a three-step process:

1. Build a platform (OS) image — this produces an `ostree_repo`.
2. Build your containers and compose app(s).
3. Combine the `ostree_repo` and your app(s) into an offline-update
   directory, ready to be uploaded per the [updates](./updates.md) guide.

## Platform build

### Using an existing FoundriesFactory

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

> **Note:** `H_BUILD` is just a build number for the platform image — pick
> any incrementing value you like. It does not need to match or correlate
> with your app build numbers; the platform and app builds are versioned
> independently and only get tied together in the [Combine](#combine) step.
> The first time you build, its recommended to set H_BUILD to your latest
> FoundriesFactory target number and add 1.

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

### Without a FoundriesFactory (meta-foundries + QLI)

> TODO: document building the platform without a FoundriesFactory.

## Build Apps

### 1. Build and push container images

Build each container image and push it to a registry, capturing the
digest of the pushed image so it can be pinned into the compose app:

```
  docker buildx build --push -t <registry>/<image-name>:<tag> .
```

The push output (or `docker/build-push-action`'s `digest` output, if
you're doing this in CI) gives you a `sha256` for the image — you'll pin
that in the next step rather than trusting a mutable tag.

### 2. Build and publish the compose app

Write a `docker-compose.yml` referencing your image(s), then publish it
with `composectl publish`, which produces the sha256 for the app itself:

```
  composectl publish -d app.hash \
    --pinned-images <registry>/<image-name>@sha256:<image-digest> \
    <registry>/<app-name>-app:<tag> amd64,arm64
```

* `-d app.hash` writes the app's own digest to the file `app.hash`.
* `--pinned-images` takes a comma-separated list of image digest URIs and
  rewrites `docker-compose.yml`'s image references to match, so you don't
  need to hand-edit digests into the compose file yourself. Omit it if
  your compose file already pins images directly.
* The trailing argument is the comma-separated list of architectures to
  publish for.

The app's own sha256 is now in `app.hash`, giving you an app URI of
`<registry>/<app-name>-app@sha256:<contents of app.hash>`.

> TODO include an example GH action

### 3. Get the app to the update server

> **Note:** the update server never needs access to the registry your apps
> or containers were pushed to. It only needs the resulting app name and
> sha256.

Pass it directly at upload time with `--apps <name>=<sha256>` (see
[Combine](#combine) below) — the simplest option when you already have the
app's sha256 (the contents of `app.hash` from step 2) sitting in CI.

Or, lay it out in an `apps` directory by pulling the published app with
`composectl pull`:

```
  composectl pull -i ./148/apps -s ./148/apps \
    <registry>/<app-name>-app@sha256:<contents of app.hash>
```

This produces the `apps/apps/<app-name>/<sha256>/` layout that
`fiocli updates upload` probes automatically — the same layout
`fioctl targets offline-update` produces for apps built through a
FoundriesFactory.

## Combine

With an `ostree_repo` from the platform build and one or more app
name/sha256 pairs from the app build run:
```
  fiocli updates upload main 148 ./148  --version 148 \
```

> **Note:** if your apps change between updates that share the same
> ostree build, don't rely on the probed `version` (from `IMAGE_VERSION`,
> which comes from your platform's `H_BUILD`) — it will collide. Pass a
> distinct `--version` for each update instead, as described in
> [updates.md](./updates.md#automatic-tuf-target-generation).

One approach to creating an app version that works for many people is:
 * Take the H_BUILD value (`ostree --repo=148/ostree_repo repo  <ref> cat /etc/os-release`) and multiply by 1000
 * Add your apps build number.

So for this example with platform build 148 and say apps version 42,
the version could become 148042.

Once combined, follow the rest of the [updates](./updates.md) guide to
upload and roll out the result.

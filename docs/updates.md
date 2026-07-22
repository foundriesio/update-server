# Updates

## Producing and Serving Update Content

Update content is the *exact* contents of what `fioctl targets
offline-update` produces (an `ostree_repo` and/or an `apps` directory,
and optionally a `tuf` directory).

### Uploading a Update

The recommended way to publish update content is
`fiocli updates upload`, which streams a local offline-update
directory to the server over the API:

```
  fiocli updates upload <tag> <update-name> <directory>
```

For example, to upload the offline content produced above and name the
update `148` under the `main` tag:

```
  fioctl targets offline-update --expires-in-days 180 --tag main intel-corei7-64-lmp-148 ./148
  # If you've enabled TUF on your server, then you should remove the `tuf` 
  # directory before running fiocli updates upload
  # rm -rf ./148/tuf
  fiocli updates upload main 148 ./148
```


The directory is archived, gzip-compressed, and streamed to the server.
This update will then show up under the "updates" view in your UI.

### Automatic TUF Target Generation

An offline-update directory produced by `fioctl` includes a `tuf`
directory with signed TUF metadata. Providing that metadata is 
*optional*: when the uploaded content does not contain a `tuf`
directory, the server generates a TUF target for the update itself
(this requires TUF to be enabled on the server).

To build the target, the server *probes* the uploaded content to make a
best-effort guess about the target's attributes:

* **`ostree-hash`** and **`name`** are read from the ostree repository's
  ref under `ostree_repo`.
* **`version`** (AppVersion) is read from `IMAGE_VERSION` in the image's
  `/usr/lib/os-release`.
* **`hardware-id`** is read from `primary_ecu_hardware_id` in the image's
  `/usr/lib/sota/conf.d/40-hardware-id.toml`. If that file is absent, the
  server falls back to detecting the architecture from the image
  (`arm64-linux` or `amd64-linux`).
* **`apps`** are discovered from the `apps` directory layout, mapping
  each app to its sha256.

Because probing is a best effort, you can override any of these attributes
with flags, which is also useful when uploading content that has no
`ostree_repo` (for example, an apps-only update):

```
  fiocli updates upload main 148 ./148 \
    --hardware-id intel-corei7-64 \
    --version 148 \
    --name intel-corei7-64-lmp-148 \
    --ostree-hash <sha256> \
    --apps shellhttpd=<sha256>
```

> [!NOTE]
> If you are bundling apps into your updates, probing the
> ostree image for a default `version` will not be sufficient. Your apps
> might change from one update to the next while still using the same
> ostree, so the probed `IMAGE_VERSION` would collide. In that case,
> supply a distinct `--version` for each update.

> [!NOTE]
> When the upload does not contain an `ostree_repo` directory,
> `--hardware-id` is required because it cannot be probed.

### Uploading via API

The upload is a `POST` to `/v1/updates/<tag>/<update>` with the
gzip-compressed tar archive as the request body. The override attributes
are passed as query parameters (`version`, `hardware-id`, `name`,
`ostree-hash`, and `apps` as `name=sha256`). For example:

```
  curl \
    -H 'Authorization: Bearer <your token>' \
    -H 'Content-Type: application/x-tar' \
    -H 'Content-Encoding: gzip' \
    -X POST \
    --data-binary @148.tar.gz \
    'http://<your server>/v1/updates/main/148?hardware-id=intel-corei7-64'
```

## Updating Your Devices

With an update in place, you will need to create a "rollout" for your
device(s) to see it. This can be done via API, CLI, or Web UI.

### API

Create a rollout named "first-try"

```
  curl \
    -H 'Authorization: Bearer <your token>' \
    -H 'Content-type: application/json' \
    -X PUT \
    -d '{"uuids": ["uuid1", "uuid2",...]}' \
    http://<your server>/v1/updates/ci/main/148/rollouts/first-try
```

### CLI

Use the `fiocli updates create-rollout` command.

### Web

Scroll down to the specific update and click "Create rollout".

## Tracking the Progress of an Update/Rollout

You can track the progress of an update through the API, CLI, or Web.

### Tracking via API

There are two API resources for tailing updates. Both resources emit
[Server Sent Events](https://en.wikipedia.org/wiki/Server-sent_events).

* **rollout** – `/v1/updates/<ci|prod>/<tag>/<update>/rollouts/<rollout>/tail`
* **the whole update** — `/v1/updates/<ci|prod>/<tag>/<update>/tail`

### Tracking via CLI

The CLI has an `updates tail` subcommand that allows you to tail the update
or a specific rollout.

### Tracking via Web

Click "Follow progress" on either the Update or Rollout to see details.

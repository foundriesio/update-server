# Foundries Update Server
The update server is an open source system for managing Foundries.io update agents.
There are two primary use cases for this project:

 * Offline (air-gapped) environments where devices can't reach the Foundries.io backend
 * Users who want to manage their own device management solution

This project handles both use cases by implementing all the APIs used by Foundries.io update agents.
The project also includes a user-facing REST API and Web UI for managing devices and updates.

## Quick start
Follow the [Quick Start](./docs/quick-start.md) guide to get a server running in development mode.

## Adding updates
The update server uses content from [Offline Updates](https://docs.foundries.io/96/user-guide/offline-update/offline-update.html)
to serve devices their TUF, OSTree, and Container data. Follow the
[updates](./docs/updates.md) guide for setting this up.

## API access
Follow the [API](./docs/api.md) to learn how to access and use the REST API.

## Configuring authentication options
Follow the [configuring authentication](./docs/auth.md) guide for chosing the
method that meets your requirements.

## Running in production
The [production guide](./docs/production.md) covers considerations when
deploying the update server for production use.

## Developing
The project is a single Golang binary that can be built with:
```
 go build -o fioserver github.com/foundriesio/update-server/cmd/server
```

A "devshell" is also included that can be used for local development:
```
 ./contrib/dev-shell
```

***NOTE***: This repository uses Git-LFS. You'll need this installed to use the web UI.

## License
*update-server* is under the [BSD 3-Clause Clear](./LICENSE.txt) license.

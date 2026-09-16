# About

This directory contains tools useful for local development

## `auth-config-*.json`

Authentication provider sample configurations for GitHub, Google, and local
username/password. See [configuring authentication](../docs/auth.md).

## `dev-shell` / `Dockerfile.devshell`

This script builds a container with all the required dependencies for
developing on this code base and will drop you in a container with the
project source code mounted.

## `e2e`

An end-to-end test suite that exercises a real update server together with
a real device client (`fioup`), driving update, config, and remote-action
flows through both the `fiocli` CLI and the web UI. See `e2e/README.md`.

## `perf-test`

Self-contained Locust-based mTLS performance test that seeds and drives
thousands of fake devices against the update server. See
`perf-test/README.md`.

## `run-local.sh`

Stands up a local development server in one step, defaulting its datadir to
`./.local-data`. See [running locally](../docs/run-locally.md).

## Terraform

Packer and Terraform recipes for a single-instance AWS deployment. See the
[production guide](../docs/production.md).

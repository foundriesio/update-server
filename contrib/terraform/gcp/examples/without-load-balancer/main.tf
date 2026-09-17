# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Update server on a single instance, no load balancers.
#
# Caddy runs on the box and terminates TLS for the UI with Let's Encrypt,
# while the device gateway is exposed directly on var.gateway_port (8443 by
# default) -- Caddy must never proxy it, because the server terminates that
# mTLS itself.
#
# One static IP serves both, so the UI and the gateway share a hostname. That
# address is baked into the gateway certificate and into every enrolled
# device's configuration, which is why assign_static_ip is not optional here.
#
# DNS must resolve to the static IP before Caddy can complete the Let's
# Encrypt HTTP-01 challenge. With managed_zone_name set, Terraform creates
# that record.

terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

module "network" {
  source = "../../modules/network"

  name_prefix        = var.name_prefix
  region             = var.region
  allowed_ssh_cidr   = var.allowed_ssh_cidr
  enable_caddy_ports = true
  gateway_port       = var.gateway_port
}

module "server" {
  source = "../../modules/server"

  project_id           = var.project_id
  name_prefix          = var.name_prefix
  hostname             = var.hostname
  image                = var.image
  region               = var.region
  zone                 = var.zone
  subnetwork_self_link = module.network.subnetwork_self_link
  server_tag           = module.network.server_tag
  gateway_port         = var.gateway_port
  machine_type         = var.machine_type
  data_volume_size     = var.data_volume_size
  ssh_public_key       = var.ssh_public_key

  enable_caddy     = true
  assign_static_ip = true

  snapshot_retention_days = var.snapshot_retention_days
  enable_cloud_logging    = var.enable_cloud_logging
  labels                  = var.labels
}

# No managed certificate in this topology -- Caddy obtains its own from
# Let's Encrypt -- so this only creates the A record pointing at the static
# IP.
module "dns" {
  source = "../../modules/dns"

  hostname          = var.hostname
  managed_zone_name = var.managed_zone_name
  ui_ip             = module.server.public_ip
}

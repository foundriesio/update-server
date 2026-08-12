# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Update server behind load balancers: a global external HTTPS LB terminates
# TLS for the UI with a Google-managed certificate, and a regional external
# passthrough Network LB passes the device gateway through untouched.
#
# The UI and the gateway need SEPARATE hostnames here, because one DNS record
# cannot point at two different reserved IPs. Devices use the gateway name.

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

  name_prefix       = var.name_prefix
  region            = var.region
  allowed_ssh_cidr  = var.allowed_ssh_cidr
  enable_lb_ingress = true
  gateway_port      = var.gateway_port
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

  # The load balancers have stable addresses; the instance doesn't need one.
  assign_static_ip = false

  snapshot_retention_days = var.snapshot_retention_days
  enable_cloud_logging    = var.enable_cloud_logging
  labels                  = var.labels
}

module "frontend" {
  source = "../../modules/frontend"

  name_prefix           = var.name_prefix
  hostname              = var.hostname
  region                = var.region
  zone                  = var.zone
  instance_name         = module.server.instance_name
  instance_ip           = module.server.internal_ip
  network_self_link     = module.network.network_self_link
  subnetwork_self_link  = module.network.subnetwork_self_link
  gateway_port          = var.gateway_port
  enable_access_logging = var.enable_access_logging
}

module "dns" {
  source = "../../modules/dns"

  hostname          = var.hostname
  gateway_hostname  = var.gateway_hostname
  managed_zone_name = var.managed_zone_name
  ui_ip             = module.frontend.ui_ip
  gateway_ip        = module.frontend.gateway_ip
}

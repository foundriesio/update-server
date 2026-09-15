# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Update server on a single instance, no load balancers.
#
# Caddy runs on the box and terminates TLS for the UI with Let's Encrypt, while
# the device gateway is exposed directly on var.gateway_port (8443 by default)
# -- Caddy must never proxy it, because the server terminates that mTLS itself.
#
# One Elastic IP serves both, so the UI and the gateway share a hostname. That
# address is baked into the gateway certificate and into every enrolled device's
# configuration, which is why the EIP is not optional here.
#
# DNS must resolve to the EIP before Caddy can complete the Let's Encrypt
# HTTP-01 challenge. With hosted_zone_id set, Terraform creates that record.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  # The network module requires two AZs for load balancer compatibility even
  # though this topology has none; only the first is used.
  azs = slice(data.aws_availability_zones.available.names, 0, 2)
}

module "network" {
  source = "../../modules/network"

  name_prefix        = var.name_prefix
  availability_zones = local.azs
  allowed_ssh_cidr   = var.allowed_ssh_cidr
  enable_caddy_ports = true
  gateway_port       = var.gateway_port
  enable_ipv6        = var.enable_ipv6
  tags               = var.tags
}

module "server" {
  source = "../../modules/server"

  name_prefix       = var.name_prefix
  hostname          = var.hostname
  gateway_port      = var.gateway_port
  ami_id            = var.ami_id
  subnet_id         = module.network.public_subnet_ids[0]
  availability_zone = local.azs[0]
  security_group_id = module.network.server_security_group_id

  instance_type    = var.instance_type
  data_volume_size = var.data_volume_size
  ssh_key_name     = var.ssh_key_name

  enable_caddy = true
  assign_eip   = true
  enable_ipv6  = var.enable_ipv6

  enable_cloudwatch_logs        = var.enable_cloudwatch_logs
  cloudwatch_log_retention_days = var.cloudwatch_log_retention_days

  snapshot_retention_days = var.snapshot_retention_days
  tags                    = var.tags
}

# No ACM certificate in this topology -- Caddy obtains its own from Let's
# Encrypt -- so this only creates the A record pointing at the Elastic IP.
module "dns" {
  source = "../../modules/dns"

  hostname           = var.hostname
  hosted_zone_id     = var.hosted_zone_id
  instance_ip        = module.server.public_ip
  instance_ipv6      = module.server.ipv6_address
  enable_ipv6        = var.enable_ipv6
  create_certificate = false

  tags = var.tags
}

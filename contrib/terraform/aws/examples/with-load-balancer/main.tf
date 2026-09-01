# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Update server behind load balancers: an ALB terminates TLS for the UI with an
# ACM certificate, and an NLB passes the device gateway through untouched.
#
# The UI and the gateway need SEPARATE hostnames here, because one DNS record
# cannot alias to two different load balancers. Devices use the gateway name.

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
  # An ALB needs two AZs; the instance and its volume live in the first.
  azs = slice(data.aws_availability_zones.available.names, 0, 2)
}

module "network" {
  source = "../../modules/network"

  name_prefix        = var.name_prefix
  availability_zones = local.azs
  allowed_ssh_cidr   = var.allowed_ssh_cidr
  enable_alb_ingress = true
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

  # The instance itself does not need a public IPv6: the dual-stack ALB/NLB
  # terminate IPv6 client connections and forward to its private IPv4
  # address. Giving the instance its own public IPv6 would let the device
  # gateway be reached directly, bypassing the NLB.
  enable_ipv6 = false

  enable_cloudwatch_logs        = var.enable_cloudwatch_logs
  cloudwatch_log_retention_days = var.cloudwatch_log_retention_days

  snapshot_retention_days = var.snapshot_retention_days
  tags                    = var.tags
}

module "dns" {
  source = "../../modules/dns"

  hostname         = var.hostname
  gateway_hostname = var.gateway_hostname
  hosted_zone_id   = var.hosted_zone_id
  enable_ipv6      = var.enable_ipv6

  alb_dns_name = module.frontend.alb_dns_name
  alb_zone_id  = module.frontend.alb_zone_id
  nlb_dns_name = module.frontend.nlb_dns_name
  nlb_zone_id  = module.frontend.nlb_zone_id

  tags = var.tags
}

module "frontend" {
  source = "../../modules/frontend"

  name_prefix           = var.name_prefix
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.public_subnet_ids
  target_ip             = module.server.private_ip
  alb_security_group_id = module.network.alb_security_group_id
  certificate_arn       = module.dns.certificate_arn
  gateway_port          = var.gateway_port
  access_logs           = var.access_logs
  enable_ipv6           = var.enable_ipv6

  tags = var.tags
}

# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Plain A records for the UI and the gateway.
#
# Simpler than the AWS module: there is no ACM-style CNAME validation dance,
# since GCP load balancer IPs are already stable reserved addresses and the
# UI's managed certificate (owned by modules/frontend, not this module) just
# needs the A record to exist and resolve.
#
# Set managed_zone_name and this module creates the records unattended. Leave
# it empty and no records are created; the records output reports what to
# create by hand -- see the README.

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
  manage_dns = var.managed_zone_name != ""

  # Key by DNS name to avoid duplicate records when both names are the same.
  records = {
    (var.hostname)                                                     = var.ui_ip
    (var.gateway_hostname == "" ? var.hostname : var.gateway_hostname) = var.gateway_ip == "" ? var.ui_ip : var.gateway_ip
  }

  # AAAA records are opt-in via enable_ipv6. Keys must be statically known at
  # plan time (Terraform restriction on for_each), so we key on the hostnames
  # and leave the IPv6 addresses as values, which may be known only after apply.
  records_v6 = var.enable_ipv6 ? {
    (var.hostname)                                                     = var.ui_ipv6
    (var.gateway_hostname == "" ? var.hostname : var.gateway_hostname) = var.gateway_ipv6
  } : {}
}

data "google_dns_managed_zone" "zone" {
  count = local.manage_dns ? 1 : 0

  name = var.managed_zone_name
}

resource "google_dns_record_set" "records" {
  for_each = local.manage_dns ? local.records : {}

  name         = "${each.key}."
  type         = "A"
  ttl          = 300
  managed_zone = data.google_dns_managed_zone.zone[0].name
  rrdatas      = [each.value]
}

resource "google_dns_record_set" "records_v6" {
  for_each = local.manage_dns ? local.records_v6 : {}

  name         = "${each.key}."
  type         = "AAAA"
  ttl          = 300
  managed_zone = data.google_dns_managed_zone.zone[0].name
  rrdatas      = [each.value]
}

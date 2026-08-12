# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Network substrate: one VPC, one subnet, and the firewall rules.
#
# Unlike the AWS module, only one region/zone is needed here: neither GCP load
# balancer topology requires the backend to span multiple zones (an ALB
# requires two AZs; GCP's global HTTPS LB and regional passthrough NLB do not).

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
  server_tag = "${var.name_prefix}-server"
}

resource "google_compute_network" "main" {
  name                    = "${var.name_prefix}-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "public" {
  name          = "${var.name_prefix}-public"
  network       = google_compute_network.main.id
  region        = var.region
  ip_cidr_range = var.subnet_cidr
}

# No egress rules: GCP VPC firewalls default-allow all egress unless a deny
# rule says otherwise, unlike AWS security groups which default-deny egress.

# IAP TCP forwarding is always allowed, mirroring the AWS module's SSM path
# being available regardless of allowed_ssh_cidr. See:
# https://cloud.google.com/iap/docs/using-tcp-forwarding
resource "google_compute_firewall" "iap_ssh" {
  name    = "${var.name_prefix}-iap-ssh"
  network = google_compute_network.main.id

  direction     = "INGRESS"
  source_ranges = ["35.235.240.0/20"]
  target_tags   = [local.server_tag]

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

resource "google_compute_firewall" "ssh_cidr" {
  count = var.allowed_ssh_cidr == "" ? 0 : 1

  name    = "${var.name_prefix}-ssh-cidr"
  network = google_compute_network.main.id

  direction     = "INGRESS"
  source_ranges = [var.allowed_ssh_cidr]
  target_tags   = [local.server_tag]

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

# The device gateway is always exposed directly. A passthrough Network LB with
# instance-group backends preserves the client's own source address -- there
# is no load-balancer CIDR to narrow this to, same as the AWS NLB path.
resource "google_compute_firewall" "gateway" {
  name    = "${var.name_prefix}-gateway"
  network = google_compute_network.main.id

  direction     = "INGRESS"
  source_ranges = ["0.0.0.0/0"]
  target_tags   = [local.server_tag]

  allow {
    protocol = "tcp"
    ports    = [tostring(var.gateway_port)]
  }
}

# The UI port is reachable only from Google's load-balancer/health-check
# ranges in the load-balancer topology. Traffic from the global external
# HTTPS LB arrives from these same ranges (it is a proxy-based LB), so this
# single rule covers both the health check and the real traffic -- there is
# no separate ALB security group to reference as in AWS.
resource "google_compute_firewall" "ui_from_lb" {
  count = var.enable_lb_ingress ? 1 : 0

  name    = "${var.name_prefix}-ui-from-lb"
  network = google_compute_network.main.id

  direction     = "INGRESS"
  source_ranges = ["130.211.0.0/22", "35.191.0.0/16"]
  target_tags   = [local.server_tag]

  allow {
    protocol = "tcp"
    ports    = ["8080"]
  }
}

# Caddy's listeners, and the HTTP-01 challenge Let's Encrypt performs against 80.
resource "google_compute_firewall" "caddy" {
  count = var.enable_caddy_ports ? 1 : 0

  name    = "${var.name_prefix}-caddy"
  network = google_compute_network.main.id

  direction     = "INGRESS"
  source_ranges = ["0.0.0.0/0"]
  target_tags   = [local.server_tag]

  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }
}

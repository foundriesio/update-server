# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "project_id" {
  type        = string
  description = "GCP project to deploy into."
}

variable "region" {
  type        = string
  description = "Region to deploy into."
  default     = "us-central1"
}

variable "zone" {
  type        = string
  description = "Zone for the instance and its data disk."
  default     = "us-central1-a"
}

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and labels."
  default     = "fioserver"
}

variable "hostname" {
  type        = string
  description = <<-EOT
    Public DNS name for both the UI (443) and the device gateway
    (var.gateway_port), e.g. "dg.example.com".

    It must resolve to this instance's static IP before Caddy can complete
    the Let's Encrypt HTTP-01 challenge. It is also baked into the gateway
    certificate and every enrolled device's configuration, so it cannot
    change later without orphaning those devices.
  EOT
}

variable "factory" {
  type        = string
  description = "Factory name recorded in the PKI subject."
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener is reachable on."
  default     = 8443
}

variable "image" {
  type        = string
  description = "Image built by contrib/terraform/gcp/packer."
}

variable "managed_zone_name" {
  type        = string
  description = <<-EOT
    Cloud DNS managed zone in which to create the A record for var.hostname.
    Leave empty to create it yourself -- but do so promptly, since Caddy
    cannot obtain a certificate until the name resolves.
  EOT
  default     = ""
}

variable "machine_type" {
  type        = string
  description = "Compute Engine machine type."
  default     = "e2-small"
}

variable "data_volume_size" {
  type        = number
  description = "Size of the persistent /data disk in GiB."
  default     = 100
}

variable "ssh_public_key" {
  type        = string
  description = "SSH public key to add via instance metadata, or \"\" to use OS Login/IAP only."
  default     = ""
}

variable "allowed_ssh_cidr" {
  type        = string
  description = "CIDR allowed to reach port 22, or \"\" to omit the rule."
  default     = ""
}

variable "tls_expiry_days" {
  type        = number
  description = "Gateway certificate validity. The gateway will not start once it expires."
  default     = 3650
}

variable "snapshot_retention_days" {
  type        = number
  description = "Days to retain daily data-disk snapshots."
  default     = 60
}

variable "enable_cloud_logging" {
  type        = bool
  description = "Ship the fioserver.service journal to Cloud Logging via the Ops Agent."
  default     = false
}

variable "labels" {
  type        = map(string)
  description = "Extra labels applied to every resource."
  default     = {}
}

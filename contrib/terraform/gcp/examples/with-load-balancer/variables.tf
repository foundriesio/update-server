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
  description = "Public DNS name for the web UI, e.g. \"dg.example.com\"."
}

variable "gateway_hostname" {
  type        = string
  description = <<-EOT
    DNS name devices use for the mTLS gateway on var.gateway_port, e.g.
    "devices.example.com".

    Must differ from var.hostname in this topology: the UI is behind the
    global HTTPS LB and the gateway behind the regional TCP LB, and a single
    DNS record cannot point at both reserved IPs.

    This name is baked into the gateway certificate and into every enrolled
    device's configuration, so it cannot be changed later without orphaning
    those devices.
  EOT

  validation {
    condition     = var.gateway_hostname != ""
    error_message = "The gateway_hostname is required and must differ from hostname."
  }
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener is reachable on."
  default     = 8443
}

variable "factory" {
  type        = string
  description = "Factory name recorded in the PKI subject."
}

variable "image" {
  type        = string
  description = "Image built by contrib/terraform/gcp/packer."
}

variable "managed_zone_name" {
  type        = string
  description = <<-EOT
    Cloud DNS managed zone for the UI and gateway A records. Leave empty to
    manage DNS yourself, but note that the UI's managed certificate stays in
    PROVISIONING until var.hostname resolves to the load balancer IP.
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

variable "enable_access_logging" {
  type        = bool
  description = "Log every UI request to Cloud Logging via the backend service's log_config."
  default     = false
}

variable "labels" {
  type        = map(string)
  description = "Extra labels applied to every resource."
  default     = {}
}

variable "enable_ipv6" {
  type        = bool
  description = <<-EOT
    Make the VM and both DNS names dual-stack. Both the UI's global HTTPS LB
    and the gateway's regional passthrough LB get a second reserved
    address/forwarding rule for IPv6. See contrib/terraform/gcp/README.md.
  EOT
  default     = false
}

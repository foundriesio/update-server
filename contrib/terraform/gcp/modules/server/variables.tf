# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "project_id" {
  type        = string
  description = "GCP project to deploy into. Required by google_project_iam_member, which -- unlike the secret-scoped IAM bindings -- has no resource to infer it from."
}

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and labels."
  default     = "fioserver"
}

variable "hostname" {
  type        = string
  description = <<-EOT
    Public DNS name browsers use for the UI, e.g. "dg.example.com". Also the
    name the UI resolves to loopback for its own internal API calls.
  EOT
}

variable "image" {
  type        = string
  description = "Custom image built by contrib/terraform/gcp/packer, e.g. \"fioserver-v0.9.2-amd64\" or its self-link."
}

variable "region" {
  type        = string
  description = "Region for the persistent disk's snapshot schedule."
}

variable "zone" {
  type        = string
  description = "Zone for the instance and its data disk."
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to launch the instance in."
}

variable "server_tag" {
  type        = string
  description = "Network tag matched by the firewall rules in modules/network."
}

variable "machine_type" {
  type        = string
  description = "Compute Engine machine type."
  default     = "e2-small"
}

variable "root_volume_size" {
  type        = number
  description = "Boot disk size in GiB. Holds only the OS; state lives on /data."
  default     = 10
}

variable "data_volume_size" {
  type        = number
  description = "Size of the persistent /data disk in GiB."
  default     = 100
}

variable "ssh_public_key" {
  type        = string
  description = "SSH public key to add via instance metadata, or \"\" to rely on OS Login/IAP only."
  default     = ""
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener binds and is reachable on."
  default     = 8443
}

variable "enable_caddy" {
  type        = bool
  description = "Run Caddy on the instance for TLS (the no-load-balancer topology)."
  default     = false
}

variable "assign_static_ip" {
  type        = bool
  description = <<-EOT
    Reserve a static external IP for the instance. Required for the Caddy
    topology: the address is baked into the TLS certificate and into every
    device's configuration, so it must not change when the instance stops.
    In the load-balancer topology the instance still gets an external IP
    (see the GCP module README for why), but it can be ephemeral.
  EOT
  default     = false
}

variable "snapshot_retention_days" {
  type        = number
  description = "Days to retain daily data-disk snapshots before deletion."
  default     = 60
}

variable "snapshot_start_time" {
  type        = string
  description = "UTC time of day for the daily snapshot, as HH:00."
  default     = "05:00"
}

variable "enable_cloud_logging" {
  type        = bool
  description = <<-EOT
    Ship the instance's systemd journal to Cloud Logging via the Ops Agent
    (installed but disabled by default in the image). Off by default since it
    adds Cloud Logging ingestion/storage cost. Unlike the AWS module's
    CloudWatch agent config, the Ops Agent's journald receiver cannot be
    scoped to a single unit, so this ships the whole journal, not just
    fioserver.service's output.
  EOT
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
    Give the instance a dual-stack NIC (internal IPv6 only -- no external
    IPv6 address on the instance itself; that's carried by modules/frontend's
    gateway forwarding rule), and bind the device gateway to it
    (FIOSERVER_GATEWAY_ADDR becomes "[::]:gateway_port" instead of
    "0.0.0.0:gateway_port"). The UI's bind address is unaffected: the global
    HTTPS LB always reconnects to the backend over IPv4 regardless of the
    client's IP family.
  EOT
  default     = false
}

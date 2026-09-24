# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and labels."
  default     = "fioserver"
}

variable "hostname" {
  type        = string
  description = "Public DNS name for the UI; also the managed certificate's domain."
}

variable "ssl_profile" {
  type        = string
  description = "SSL policy profile for the UI's HTTPS proxy."
  default     = "RESTRICTED"
}

variable "ssl_min_tls_version" {
  type        = string
  description = "Minimum TLS version for the UI's HTTPS proxy."
  default     = "TLS_1_2"
}

variable "region" {
  type        = string
  description = "Region for the gateway's regional load balancer."
}

variable "zone" {
  type        = string
  description = "Zone of the update server instance, for the instance group."
}

variable "instance_name" {
  type        = string
  description = "Name of the update server instance, for the network endpoints."
}

variable "instance_ip" {
  type        = string
  description = "Internal IP of the update server instance, for the network endpoints."
}

variable "instance_self_link" {
  type        = string
  description = "Self-link of the update server instance, for the gateway's instance group."
}

variable "network_self_link" {
  type        = string
  description = "Self-link of the VPC network, for the network endpoint groups."
}

variable "subnetwork_self_link" {
  type        = string
  description = "Self-link of the subnetwork, for the network endpoint groups."
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener is reachable on."
  default     = 8443
}

variable "timeout_sec" {
  type        = number
  description = <<-EOT
    UI backend service timeout in seconds. Uploading a large update through
    the REST API can hold a connection open for a while, so this is generous
    by default.
  EOT
  default     = 300
}

variable "enable_access_logging" {
  type        = bool
  description = "Log every UI request to Cloud Logging via the backend service's log_config."
  default     = false
}

variable "enable_ipv6" {
  type        = bool
  description = <<-EOT
    Add a second reserved address/forwarding-rule pair for each load
    balancer so both the UI and the gateway are reachable over IPv6.

    The UI's global HTTPS LB (GFE) always reconnects to its backend over
    IPv4 regardless of the client's IP family, so its backend
    service/NEG/health-check are reused unchanged.

    The gateway's regional passthrough LB has no such proxying layer, so its
    backend must itself support an IPv6 endpoint -- which is why the gateway
    uses an unmanaged instance group rather than a NEG (see
    google_compute_instance_group.gateway in main.tf): NEGs have no IPv6
    endpoint type on this LB, but instance groups do.
  EOT
  default     = false
}

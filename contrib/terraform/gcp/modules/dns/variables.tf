# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "hostname" {
  type        = string
  description = "Public DNS name for the UI."
}

variable "gateway_hostname" {
  type        = string
  description = <<-EOT
    DNS name for the mTLS gateway. Required when load balancers are used,
    since the global HTTPS LB and the regional TCP LB get separate reserved
    IPs and cannot share one A record. Empty means the gateway shares
    var.hostname (correct for the Caddy topology, where both are the same
    instance IP anyway).
  EOT
  default     = ""
}

variable "managed_zone_name" {
  type        = string
  description = <<-EOT
    Cloud DNS managed zone to create records in.

    Empty means no records are created; use the module's records output to
    create them by hand. The UI's Google-managed certificate (see
    modules/frontend) stays in PROVISIONING until its A record resolves to
    ui_ip, so in the load-balancer topology that record must exist before the
    certificate can issue.
  EOT
  default     = ""
}

variable "ui_ip" {
  type        = string
  description = "Address for the UI's A record: the global LB IP, or the instance's own IP in the Caddy topology."
}

variable "gateway_ip" {
  type        = string
  description = <<-EOT
    Address for the gateway's A record: the regional LB IP in the
    load-balancer topology. Empty means the gateway shares ui_ip (correct for
    the Caddy topology).
  EOT
  default     = ""
}

variable "ui_ipv6" {
  type        = string
  description = "Address for the UI's AAAA record, or \"\" to omit it (the default -- enable_ipv6 is off upstream)."
  default     = ""
}

variable "gateway_ipv6" {
  type        = string
  description = <<-EOT
    Address for the gateway's AAAA record, or "" to omit it. In the
    load-balancer topology this is the regional passthrough LB's reserved
    IPv6 address (see modules/frontend).
  EOT
  default     = ""
}

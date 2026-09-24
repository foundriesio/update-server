# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and tags."
  default     = "fioserver"
}

variable "region" {
  type        = string
  description = "Region for the subnet."
}

variable "subnet_cidr" {
  type        = string
  description = "CIDR block for the single subnet."
  default     = "10.0.0.0/20"
}

variable "allowed_ssh_cidr" {
  type        = string
  description = <<-EOT
    CIDR permitted to reach port 22 directly. Set to "" to omit the rule
    entirely and rely on IAP TCP forwarding, which is the recommended access
    path and is always allowed regardless of this variable.
  EOT
  default     = ""
}

variable "enable_lb_ingress" {
  type        = bool
  description = "Allow Google's load-balancer/health-check ranges to reach the instance's UI port."
  default     = false
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener is reachable on."
  default     = 8443
}

variable "enable_ipv6" {
  type        = bool
  description = <<-EOT
    Make the subnet dual-stack and allow IPv6 clients to reach the device
    gateway. In the load-balancer topology this also carves out the
    external IPv6 range modules/frontend's gateway forwarding rule reserves
    an address from.
  EOT
  default     = false
}

# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and tags."
  default     = "fioserver"
}

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC."
  default     = "10.0.0.0/16"
}

variable "availability_zones" {
  type        = list(string)
  description = <<-EOT
    Availability zones for the public subnets. At least two are required: an
    ALB will not launch into a single AZ, even though only one instance runs.
  EOT

  validation {
    condition     = length(var.availability_zones) >= 2
    error_message = "At least two availability zones are required for the load balancers."
  }
}

variable "allowed_ssh_cidr" {
  type        = string
  description = <<-EOT
    CIDR permitted to reach port 22. Set to "" to omit the rule entirely and
    rely on SSM Session Manager, which is the recommended access path.
  EOT
  default     = ""
}

variable "enable_alb_ingress" {
  type        = bool
  description = "Allow the ALB security group to reach the instance's UI port."
  default     = false
}

variable "enable_caddy_ports" {
  type        = bool
  description = "Open 80/443 for Caddy and its ACME HTTP-01 challenge."
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
    Assign the VPC an Amazon-provided IPv6 CIDR, give each public subnet a
    /64, route ::/0 through the internet gateway, and open the equivalent
    ::/0 security-group rules alongside the existing 0.0.0.0/0 ones.
  EOT
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Extra tags applied to every resource."
  default     = {}
}

# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "aws_region" {
  type        = string
  description = "AWS region to deploy into."
  default     = "us-east-1"
}

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and tags."
  default     = "fioserver"
}

variable "hostname" {
  type        = string
  description = <<-EOT
    Public DNS name for both the UI (443) and the device gateway
    (var.gateway_port), e.g. "dg.example.com".

    It must resolve to this instance's Elastic IP before Caddy can complete the
    Let's Encrypt HTTP-01 challenge. It is also baked into the gateway
    certificate and every enrolled device's configuration, so it cannot change
    later without orphaning those devices.
  EOT
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener is reachable on."
  default     = 8443
}

variable "enable_ipv6" {
  type        = bool
  description = <<-EOT
    Give the instance a public IPv6 address in addition to its Elastic IP,
    and create an AAAA record alongside the A record. Elastic IPs are
    IPv4-only, so IPv6 reachability here is the instance's own address, not
    an EIP -- it stays stable across reboots because it's tied to the ENI.
  EOT
  default     = true
}

variable "ami_id" {
  type        = string
  description = "AMI built by contrib/terraform/aws/packer."
}

variable "hosted_zone_id" {
  type        = string
  description = <<-EOT
    Route53 zone in which to create the A record for var.hostname. Leave empty
    to create it yourself -- but do so promptly, since Caddy cannot obtain a
    certificate until the name resolves.
  EOT
  default     = ""
}

variable "instance_type" {
  type        = string
  description = "EC2 instance type."
  default     = "t3.small"
}

variable "data_volume_size" {
  type        = number
  description = "Size of the persistent /data volume in GiB."
  default     = 100
}

variable "ssh_key_name" {
  type        = string
  description = "Existing EC2 key pair, or \"\" to use SSM Session Manager only."
  default     = ""
}

variable "allowed_ssh_cidr" {
  type        = string
  description = "CIDR allowed to reach port 22, or \"\" to omit the rule."
  default     = ""
}

variable "snapshot_retention_days" {
  type        = number
  description = "Days to retain daily data-volume snapshots."
  default     = 60
}

variable "enable_cloudwatch_logs" {
  type        = bool
  description = "Ship the fioserver.service journal to CloudWatch Logs via the CloudWatch agent."
  default     = false
}

variable "cloudwatch_log_retention_days" {
  type        = number
  description = "Retention for the CloudWatch log group, when enable_cloudwatch_logs is set."
  default     = 30
}

variable "tags" {
  type        = map(string)
  description = "Extra tags applied to every resource."
  default     = {}
}

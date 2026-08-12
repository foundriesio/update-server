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
  description = "Public DNS name for the web UI, e.g. \"dg.example.com\"."
}

variable "gateway_hostname" {
  type        = string
  description = <<-EOT
    DNS name devices use for the mTLS gateway on var.gateway_port, e.g.
    "devices.example.com".

    Must differ from var.hostname in this topology: the UI is behind an ALB and
    the gateway behind an NLB, and a single DNS record cannot alias to both.

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

variable "enable_ipv6" {
  type        = bool
  description = "Deploy dual-stack: VPC/subnets get an IPv6 CIDR, the ALB/NLB run dual-stack, and AAAA records are created alongside the A records."
  default     = true
}

variable "ami_id" {
  type        = string
  description = "AMI built by contrib/terraform/aws/packer."
}

variable "hosted_zone_id" {
  type        = string
  description = <<-EOT
    Route53 zone for the certificate validation and alias records. Leave empty
    to manage DNS yourself, but note that apply then blocks until the ACM
    validation record exists.
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

variable "access_logs" {
  type = object({
    bucket  = string
    prefix  = optional(string, "")
    enabled = optional(bool, true)
  })
  description = <<-EOT
    S3 access logging for the UI ALB. The bucket must already exist and grant
    the ELB service account write access. Leave unset (null) to disable.
  EOT
  default     = null
}

variable "tags" {
  type        = map(string)
  description = "Extra tags applied to every resource."
  default     = {}
}

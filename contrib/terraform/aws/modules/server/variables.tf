# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and tags."
  default     = "fioserver"
}

variable "hostname" {
  type        = string
  description = <<-EOT
    Public DNS name browsers use for the UI, e.g. "dg.example.com". Also the
    name the UI resolves to loopback for its own internal API calls.
  EOT
}

variable "ami_id" {
  type        = string
  description = "AMI built by contrib/terraform/aws/packer."
}

variable "subnet_id" {
  type        = string
  description = "Subnet to launch the instance in."
}

variable "availability_zone" {
  type        = string
  description = "AZ for the data volume; must match the subnet's AZ."
}

variable "security_group_id" {
  type        = string
  description = "Security group for the instance."
}

variable "instance_type" {
  type        = string
  description = "EC2 instance type."
  default     = "t3.small"
}

variable "root_volume_size" {
  type        = number
  description = "Root volume size in GiB. Holds only the OS; state lives on /data."
  default     = 8
}

variable "data_volume_size" {
  type        = number
  description = "Size of the persistent /data volume in GiB."
  default     = 100
}

variable "ssh_key_name" {
  type        = string
  description = "Existing EC2 key pair name, or \"\" to rely on SSM only."
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

variable "assign_eip" {
  type        = bool
  description = <<-EOT
    Attach an Elastic IP. Required for the Caddy topology: the address is baked
    into the TLS certificate and into every device's configuration, so it must
    not change when the instance stops.
  EOT
  default     = false
}

variable "enable_ssm" {
  type        = bool
  description = "Attach AmazonSSMManagedInstanceCore for Session Manager access."
  default     = true
}

variable "snapshot_retention_days" {
  type        = number
  description = "Days to retain daily data-volume snapshots before deletion."
  default     = 60
}

variable "snapshot_start_time" {
  type        = string
  description = "UTC time of day for the daily snapshot, as HH:MM."
  default     = "05:17"
}

variable "enable_cloudwatch_logs" {
  type        = bool
  description = <<-EOT
    Ship the fioserver.service journal to CloudWatch Logs via the CloudWatch
    agent (installed but disabled by default in the AMI). Off by default
    since it adds CloudWatch Logs ingestion/storage cost.
  EOT
  default     = false
}

variable "cloudwatch_log_retention_days" {
  type        = number
  description = "Retention for the CloudWatch log group, when enable_cloudwatch_logs is set."
  default     = 30
}

variable "enable_ipv6" {
  type        = bool
  description = "Assign the instance a public IPv6 address in addition to IPv4."
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Extra tags applied to every resource."
  default     = {}
}

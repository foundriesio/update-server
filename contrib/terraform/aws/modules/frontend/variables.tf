# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "name_prefix" {
  type        = string
  description = "Prefix applied to resource names and tags."
  default     = "fioserver"
}

variable "vpc_id" {
  type        = string
  description = "VPC the target groups belong to."
}

variable "subnet_ids" {
  type        = list(string)
  description = "Public subnets for the load balancers; at least two AZs."
}

variable "target_ip" {
  type        = string
  description = "Private IP of the update server instance."
}

variable "alb_security_group_id" {
  type        = string
  description = "Security group for the ALB."
}

variable "certificate_arn" {
  type        = string
  description = "ACM certificate ARN for the HTTPS listener."
}

variable "ssl_policy" {
  type        = string
  description = "ALB SSL policy for the HTTPS listener."
  default     = "ELBSecurityPolicy-TLS13-1-2-2021-06"
}

variable "idle_timeout" {
  type        = number
  description = <<-EOT
    ALB idle timeout in seconds. Uploading a large update through the REST API
    can hold a connection open for a while, so this is generous by default.
  EOT
  default     = 300
}

variable "gateway_port" {
  type        = number
  description = "Port the device gateway's mTLS listener is reachable on."
  default     = 8443
}

variable "enable_ipv6" {
  type        = bool
  description = "Run the ALB and NLB dual-stack (IPv4 and IPv6). Targets stay IPv4."
  default     = true
}

variable "access_logs" {
  type = object({
    bucket  = string
    prefix  = optional(string, "")
    enabled = optional(bool, true)
  })
  description = <<-EOT
    S3 access logging for the UI ALB. The bucket must already exist and grant
    the ELB service account write access -- Terraform does not create it.
    Leave unset (null) to disable access logging.
  EOT
  default     = null
}

variable "tags" {
  type        = map(string)
  description = "Extra tags applied to every resource."
  default     = {}
}

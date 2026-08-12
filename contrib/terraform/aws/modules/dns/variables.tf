# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

variable "hostname" {
  type        = string
  description = "Public DNS name for the UI; also the ACM certificate's subject."
}

variable "gateway_hostname" {
  type        = string
  description = <<-EOT
    DNS name for the mTLS gateway. Required when load balancers are used, since
    the ALB and NLB cannot share one A record. Empty means the gateway shares
    var.hostname (correct for the Caddy topology).
  EOT
  default     = ""
}

variable "hosted_zone_id" {
  type        = string
  description = <<-EOT
    Route53 hosted zone to manage records in.

    Empty means Terraform creates the ACM certificate but no records. Apply will
    then BLOCK in aws_acm_certificate_validation until you create the CNAME
    reported by the validation_record output. Consider a targeted apply of the
    certificate first:
      terraform apply -target=module.dns.aws_acm_certificate.ui
  EOT
  default     = ""
}

variable "create_certificate" {
  type        = bool
  description = <<-EOT
    Request an ACM certificate for var.hostname. True for the load-balancer
    topology, where the ALB terminates TLS.

    False for the Caddy topology: Caddy obtains its own certificate from Let's
    Encrypt, and requesting an unused ACM certificate would leave apply blocked
    on a validation step nothing consumes.
  EOT
  default     = true
}

variable "alb_dns_name" {
  type        = string
  description = "ALB DNS name to alias the UI record to. Empty to skip."
  default     = ""
}

variable "alb_zone_id" {
  type        = string
  description = "ALB hosted zone ID."
  default     = ""
}

variable "nlb_dns_name" {
  type        = string
  description = "NLB DNS name to alias the gateway record to. Empty to skip."
  default     = ""
}

variable "nlb_zone_id" {
  type        = string
  description = "NLB hosted zone ID."
  default     = ""
}

variable "instance_ip" {
  type        = string
  description = <<-EOT
    Elastic IP for a direct A record, used by the Caddy topology. Mutually
    exclusive with the load balancer aliases.
  EOT
  default     = ""
}

variable "instance_ipv6" {
  type        = string
  description = <<-EOT
    Instance's public IPv6 address for a direct AAAA record, used by the
    Caddy topology. Leave empty to skip the AAAA record.
  EOT
  default     = ""
}

variable "enable_ipv6" {
  type        = bool
  description = "Create AAAA records alongside the A records."
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Extra tags applied to every resource."
  default     = {}
}

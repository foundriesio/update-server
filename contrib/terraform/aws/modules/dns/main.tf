# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# The ACM certificate for the UI, and optionally the Route53 records.
#
# Set hosted_zone_id and this module creates the validation records and the
# alias records, so `terraform apply` completes unattended. Leave it empty and
# the certificate is still requested, but apply BLOCKS in the validation step
# until you create the CNAME by hand -- the record name and value are exposed as
# outputs for exactly that. See the README.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }
}

locals {
  manage_dns = var.hosted_zone_id != ""

  # Must be knowable at plan time, so it is an explicit input rather than being
  # inferred from instance_ip -- that value is only known after the EIP exists,
  # which would make this count unresolvable during plan.
  want_certificate = var.create_certificate
}

# Only the UI name needs an ACM certificate: it is the ALB that terminates TLS.
# The gateway presents its OWN certificate, minted by pki-init on the instance
# and never seen by ACM, because the NLB passes 8443 straight through.
resource "aws_acm_certificate" "ui" {
  count = local.want_certificate ? 1 : 0

  domain_name       = var.hostname
  validation_method = "DNS"

  tags = merge(var.tags, { Name = var.hostname })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "validation" {
  for_each = local.manage_dns && local.want_certificate ? {
    for dvo in aws_acm_certificate.ui[0].domain_validation_options :
    dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  } : {}

  zone_id         = var.hosted_zone_id
  name            = each.value.name
  type            = each.value.type
  records         = [each.value.record]
  ttl             = 60
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "ui" {
  count = local.want_certificate ? 1 : 0

  certificate_arn         = aws_acm_certificate.ui[0].arn
  validation_record_fqdns = local.manage_dns ? [for r in aws_route53_record.validation : r.fqdn] : null
}

# The UI record. Both ports live on the same hostname -- 443 reaches the ALB and
# 8443 the NLB -- so when load balancers are in play the A record points at the
# ALB and a second name is used for the gateway.
#
# Gated on create_certificate, which is static: the LB DNS names are not known
# until apply and so cannot drive a count.
resource "aws_route53_record" "ui" {
  count = local.manage_dns && local.want_certificate ? 1 : 0

  zone_id = var.hosted_zone_id
  name    = var.hostname
  type    = "A"

  alias {
    name                   = var.alb_dns_name
    zone_id                = var.alb_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "ui_ipv6" {
  count = local.manage_dns && local.want_certificate && var.enable_ipv6 ? 1 : 0

  zone_id = var.hosted_zone_id
  name    = var.hostname
  type    = "AAAA"

  alias {
    name                   = var.alb_dns_name
    zone_id                = var.alb_zone_id
    evaluate_target_health = false
  }
}

# The gateway needs its own name, because a single A record cannot point at two
# different load balancers. Devices are told this name via the TLS certificate's
# SAN, so it must match the hostname passed to pki-init.
resource "aws_route53_record" "gateway" {
  count = local.manage_dns && local.want_certificate ? 1 : 0

  zone_id = var.hosted_zone_id
  name    = var.gateway_hostname == "" ? var.hostname : var.gateway_hostname
  type    = "A"

  alias {
    name                   = var.nlb_dns_name
    zone_id                = var.nlb_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "gateway_ipv6" {
  count = local.manage_dns && local.want_certificate && var.enable_ipv6 ? 1 : 0

  zone_id = var.hosted_zone_id
  name    = var.gateway_hostname == "" ? var.hostname : var.gateway_hostname
  type    = "AAAA"

  alias {
    name                   = var.nlb_dns_name
    zone_id                = var.nlb_zone_id
    evaluate_target_health = false
  }
}

# The Caddy topology: one address for everything, so a plain A record to the EIP.
# Gated on create_certificate rather than on instance_ip, which is not known
# until apply.
resource "aws_route53_record" "direct" {
  count = local.manage_dns && !local.want_certificate ? 1 : 0

  zone_id = var.hosted_zone_id
  name    = var.hostname
  type    = "A"
  ttl     = 300
  records = [var.instance_ip]
}

resource "aws_route53_record" "direct_ipv6" {
  count = local.manage_dns && !local.want_certificate && var.enable_ipv6 && var.instance_ipv6 != "" ? 1 : 0

  zone_id = var.hosted_zone_id
  name    = var.hostname
  type    = "AAAA"
  ttl     = 300
  records = [var.instance_ipv6]
}

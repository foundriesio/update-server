# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Network substrate: one VPC, two public subnets, and the security groups.
#
# Two subnets even though only one instance runs: an ALB requires subnets in at
# least two availability zones. The instance lives in the first of them.

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
  tags = merge(var.tags, { Name = var.name_prefix })
}

resource "aws_vpc" "main" {
  cidr_block                       = var.vpc_cidr
  enable_dns_hostnames             = true
  enable_dns_support               = true
  assign_generated_ipv6_cidr_block = var.enable_ipv6

  tags = merge(local.tags, { Name = "${var.name_prefix}-vpc" })
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = merge(local.tags, { Name = "${var.name_prefix}-igw" })
}

resource "aws_subnet" "public" {
  count = length(var.availability_zones)

  vpc_id                          = aws_vpc.main.id
  availability_zone               = var.availability_zones[count.index]
  cidr_block                      = cidrsubnet(var.vpc_cidr, 8, count.index)
  map_public_ip_on_launch         = !var.enable_alb_ingress
  ipv6_cidr_block                 = var.enable_ipv6 ? cidrsubnet(aws_vpc.main.ipv6_cidr_block, 8, count.index) : null
  assign_ipv6_address_on_creation = var.enable_ipv6

  tags = merge(local.tags, { Name = "${var.name_prefix}-public-${var.availability_zones[count.index]}" })
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  dynamic "route" {
    for_each = var.enable_ipv6 ? [1] : []
    content {
      ipv6_cidr_block = "::/0"
      gateway_id      = aws_internet_gateway.main.id
    }
  }

  tags = merge(local.tags, { Name = "${var.name_prefix}-public" })
}

resource "aws_route_table_association" "public" {
  count = length(aws_subnet.public)

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# --------------------------------------------------------------------- ALB ----
# Only created in the load-balancer topology; harmless otherwise, but the
# example that does not use an ALB simply never references it.
resource "aws_security_group" "alb" {
  name_prefix = "${var.name_prefix}-alb-"
  description = "Public HTTP/HTTPS ingress for the update server UI"
  vpc_id      = aws_vpc.main.id

  tags = merge(local.tags, { Name = "${var.name_prefix}-alb" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTP, redirected to HTTPS"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "alb_http_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.alb.id
  description       = "HTTP, redirected to HTTPS (IPv6)"
  cidr_ipv6         = "::/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTPS"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "alb_https_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.alb.id
  description       = "HTTPS (IPv6)"
  cidr_ipv6         = "::/0"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "alb_all" {
  security_group_id = aws_security_group.alb.id
  description       = "Forward to the instance"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

resource "aws_vpc_security_group_egress_rule" "alb_all_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.alb.id
  description       = "Forward to the instance (IPv6)"
  cidr_ipv6         = "::/0"
  ip_protocol       = "-1"
}

# ------------------------------------------------------------------ server ----
resource "aws_security_group" "server" {
  name_prefix = "${var.name_prefix}-server-"
  description = "Update server instance"
  vpc_id      = aws_vpc.main.id

  tags = merge(local.tags, { Name = "${var.name_prefix}-server" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_egress_rule" "server_all" {
  security_group_id = aws_security_group.server.id
  description       = "Outbound: GitHub releases, ACME, Secrets Manager"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

resource "aws_vpc_security_group_egress_rule" "server_all_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.server.id
  description       = "Outbound: GitHub releases, ACME, Secrets Manager (IPv6)"
  cidr_ipv6         = "::/0"
  ip_protocol       = "-1"
}

# The device gateway is always exposed directly. An NLB with IP targets performs
# no source NAT, so the client address reaching the instance is the device's own
# -- there is no load balancer CIDR to narrow this to.
resource "aws_vpc_security_group_ingress_rule" "server_gateway" {
  security_group_id = aws_security_group.server.id
  description       = "Device gateway mTLS (server terminates TLS itself)"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = var.gateway_port
  to_port           = var.gateway_port
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "server_gateway_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.server.id
  description       = "Device gateway mTLS (server terminates TLS itself) (IPv6)"
  cidr_ipv6         = "::/0"
  from_port         = var.gateway_port
  to_port           = var.gateway_port
  ip_protocol       = "tcp"
}

# The UI port is reachable only from the ALB in the load-balancer topology. In
# the Caddy topology nothing external reaches 8080 at all: the server binds
# loopback and Caddy proxies to it.
resource "aws_vpc_security_group_ingress_rule" "server_ui_from_alb" {
  count = var.enable_alb_ingress ? 1 : 0

  security_group_id            = aws_security_group.server.id
  description                  = "UI/REST from the ALB only"
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}

# Caddy's listeners, and the HTTP-01 challenge Let's Encrypt performs against 80.
resource "aws_vpc_security_group_ingress_rule" "server_caddy_http" {
  count = var.enable_caddy_ports ? 1 : 0

  security_group_id = aws_security_group.server.id
  description       = "Caddy HTTP and the ACME HTTP-01 challenge"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "server_caddy_http_ipv6" {
  count = var.enable_caddy_ports && var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.server.id
  description       = "Caddy HTTP and the ACME HTTP-01 challenge (IPv6)"
  cidr_ipv6         = "::/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "server_caddy_https" {
  count = var.enable_caddy_ports ? 1 : 0

  security_group_id = aws_security_group.server.id
  description       = "Caddy HTTPS"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "server_caddy_https_ipv6" {
  count = var.enable_caddy_ports && var.enable_ipv6 ? 1 : 0

  security_group_id = aws_security_group.server.id
  description       = "Caddy HTTPS (IPv6)"
  cidr_ipv6         = "::/0"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "server_ssh" {
  count = var.allowed_ssh_cidr == "" ? 0 : 1

  security_group_id = aws_security_group.server.id
  description       = "SSH"
  cidr_ipv4         = var.allowed_ssh_cidr
  from_port         = 22
  to_port           = 22
  ip_protocol       = "tcp"
}

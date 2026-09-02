# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Two load balancers, because the two ports have incompatible requirements.
#
# ALB for the UI (8080). It has to be an ALB rather than an NLB TLS listener:
# the web UI calls its own REST API over the network, building the base URL from
# the request scheme, and the UI port is plain HTTP. An L4 TLS listener injects
# no headers, so the server would see "http", emit http:// URLs and self-call
# port 80. An ALB sets X-Forwarded-Proto natively. Session and CSRF cookies are
# also Secure+SameSite=Strict, so the browser-facing origin must be HTTPS.
#
# NLB for the device gateway (var.gateway_port), TCP passthrough only. The
# server terminates mTLS itself and needs the device's client certificate
# intact; terminating that at an L7 proxy would discard it.

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

# -------------------------------------------------------------------- ALB ----
resource "aws_lb" "ui" {
  name_prefix        = "fio-"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [var.alb_security_group_id]
  subnets            = var.subnet_ids

  idle_timeout = var.idle_timeout

  dynamic "access_logs" {
    for_each = var.access_logs != null ? [var.access_logs] : []
    content {
      bucket  = access_logs.value.bucket
      prefix  = access_logs.value.prefix
      enabled = access_logs.value.enabled
    }
  }

  tags = merge(local.tags, { Name = "${var.name_prefix}-ui" })
}

resource "aws_lb_target_group" "ui" {
  name_prefix = "fio-"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id

  # IP targets rather than instance IDs: this keeps the target independent of
  # instance replacement and avoids NLB loopback restrictions on the gateway
  # group below.
  target_type = "ip"

  health_check {
    path                = "/healthz"
    matcher             = "200"
    protocol            = "HTTP"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = merge(local.tags, { Name = "${var.name_prefix}-ui" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_lb_target_group_attachment" "ui" {
  target_group_arn = aws_lb_target_group.ui.arn
  target_id        = var.target_ip
  port             = 8080
}

resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.ui.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }

  tags = local.tags
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.ui.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = var.ssl_policy
  certificate_arn   = var.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.ui.arn
  }

  tags = local.tags
}

# -------------------------------------------------------------------- NLB ----
resource "aws_lb" "gateway" {
  name_prefix        = "fio-"
  internal           = false
  load_balancer_type = "network"
  subnets            = var.subnet_ids

  enable_cross_zone_load_balancing = true

  tags = merge(local.tags, { Name = "${var.name_prefix}-gateway" })
}

resource "aws_lb_target_group" "gateway" {
  name_prefix = "fio-"
  port        = var.gateway_port
  protocol    = "TCP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  # A TCP check is all that is possible here, and all that is needed: the
  # gateway answers an unauthenticated request with 403 rather than closing the
  # connection, so a completed handshake proves it is serving.
  health_check {
    protocol            = "HTTPS"
    path                = "/healthz"
    matcher             = "200"
    interval            = 30
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = merge(local.tags, { Name = "${var.name_prefix}-gateway" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_lb_target_group_attachment" "gateway" {
  target_group_arn = aws_lb_target_group.gateway.arn
  target_id        = var.target_ip
  port             = var.gateway_port
}

resource "aws_lb_listener" "gateway" {
  load_balancer_arn = aws_lb.gateway.arn
  port              = var.gateway_port
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.gateway.arn
  }

  tags = local.tags
}

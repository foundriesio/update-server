# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Two load balancers, because the two ports have incompatible requirements.
#
# Global external HTTPS LB for the UI (8080). It has to be an HTTPS LB rather
# than a TCP passthrough: the web UI calls its own REST API over the network,
# building the base URL from the request scheme, and the UI port is plain
# HTTP. A TCP listener injects no headers, so the server would see "http",
# emit http:// URLs and self-call port 80. The HTTPS LB sets X-Forwarded-Proto
# natively. Session and CSRF cookies are also Secure+SameSite=Strict, so the
# browser-facing origin must be HTTPS.
#
# Regional external passthrough Network LB for the device gateway
# (var.gateway_port), TCP passthrough only. The server terminates mTLS itself
# and needs the device's client certificate intact; terminating that at an L7
# proxy would discard it.

terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

# Same as the AWS module reusing one target_ip across the ALB and NLB target
# groups, except GCP won't let a single VM belong to two load-balanced
# instance groups at all ("instance may belong to at most one load-balanced
# instance group"). Zonal NEGs don't have that restriction, so each backend
# service gets its own NEG pointed at the same instance instead.
resource "google_compute_network_endpoint_group" "ui" {
  name                  = "${var.name_prefix}-ui"
  zone                  = var.zone
  network               = var.network_self_link
  subnetwork            = var.subnetwork_self_link
  network_endpoint_type = "GCE_VM_IP_PORT"
  default_port          = 8080
}

resource "google_compute_network_endpoint" "ui" {
  network_endpoint_group = google_compute_network_endpoint_group.ui.name
  zone                   = var.zone
  instance               = var.instance_name
  ip_address             = var.instance_ip
  port                   = 8080
}

# GCE_VM_IP, not GCE_VM_IP_PORT: the passthrough Network LB forwards
# gateway_port untouched, so the endpoint has no port of its own to carry.
resource "google_compute_network_endpoint_group" "gateway" {
  name                  = "${var.name_prefix}-gateway"
  zone                  = var.zone
  network               = var.network_self_link
  subnetwork            = var.subnetwork_self_link
  network_endpoint_type = "GCE_VM_IP"
}

resource "google_compute_network_endpoint" "gateway" {
  network_endpoint_group = google_compute_network_endpoint_group.gateway.name
  zone                   = var.zone
  instance               = var.instance_name
  ip_address             = var.instance_ip
}

# --------------------------------------------------------------------- UI ----
resource "google_compute_global_address" "ui" {
  name = "${var.name_prefix}-ui"
}

resource "google_compute_health_check" "ui" {
  name = "${var.name_prefix}-ui"

  # /favicon, not /. The root path is wrapped in a session check and its
  # status depends on the configured auth provider -- 307 under noauth, 401
  # under local auth. /favicon returns 200 unauthenticated under every
  # provider, so the check does not have to change with the operator's auth
  # configuration.
  http_health_check {
    port         = 8080
    request_path = "/favicon"
  }

  check_interval_sec  = 30
  timeout_sec         = 5
  healthy_threshold   = 2
  unhealthy_threshold = 3
}

resource "google_compute_backend_service" "ui" {
  name                  = "${var.name_prefix}-ui"
  protocol              = "HTTP"
  timeout_sec           = var.timeout_sec
  load_balancing_scheme = "EXTERNAL"
  health_checks         = [google_compute_health_check.ui.id]

  backend {
    group                 = google_compute_network_endpoint_group.ui.id
    balancing_mode        = "RATE"
    max_rate_per_endpoint = 100
  }

  dynamic "log_config" {
    for_each = var.enable_access_logging ? [1] : []
    content {
      enable      = true
      sample_rate = 1.0
    }
  }
}

resource "google_compute_managed_ssl_certificate" "ui" {
  name = "${var.name_prefix}-ui"

  managed {
    domains = [var.hostname]
  }
}

resource "google_compute_url_map" "ui" {
  name            = "${var.name_prefix}-ui"
  default_service = google_compute_backend_service.ui.id
}

resource "google_compute_ssl_policy" "ui" {
  name            = "${var.name_prefix}-ui"
  profile         = var.ssl_profile
  min_tls_version = var.ssl_min_tls_version
}

resource "google_compute_target_https_proxy" "ui" {
  name             = "${var.name_prefix}-ui"
  url_map          = google_compute_url_map.ui.id
  ssl_certificates = [google_compute_managed_ssl_certificate.ui.id]
  ssl_policy       = google_compute_ssl_policy.ui.id
}

resource "google_compute_global_forwarding_rule" "ui_https" {
  name                  = "${var.name_prefix}-ui-https"
  target                = google_compute_target_https_proxy.ui.id
  ip_address            = google_compute_global_address.ui.address
  port_range            = "443"
  load_balancing_scheme = "EXTERNAL"
}

# google_compute_managed_ssl_certificate.ui above has no challenge of its
# own to serve: Google just polls the hostname's A record (see modules/dns)
# until it resolves to this address, then issues. This redirect only exists
# for plain-HTTP visitors, not to satisfy the certificate.
resource "google_compute_url_map" "ui_redirect" {
  name = "${var.name_prefix}-ui-redirect"

  default_url_redirect {
    https_redirect = true
    strip_query    = false
  }
}

resource "google_compute_target_http_proxy" "ui_redirect" {
  name    = "${var.name_prefix}-ui-redirect"
  url_map = google_compute_url_map.ui_redirect.id
}

resource "google_compute_global_forwarding_rule" "ui_http" {
  name                  = "${var.name_prefix}-ui-http"
  target                = google_compute_target_http_proxy.ui_redirect.id
  ip_address            = google_compute_global_address.ui.address
  port_range            = "80"
  load_balancing_scheme = "EXTERNAL"
}

# ---------------------------------------------------------------- gateway ----
resource "google_compute_address" "gateway" {
  name   = "${var.name_prefix}-gateway"
  region = var.region
}

resource "google_compute_region_health_check" "gateway" {
  name   = "${var.name_prefix}-gateway"
  region = var.region

  # A TCP check is all that is possible here, and all that is needed: the
  # gateway answers an unauthenticated request with 403 rather than closing
  # the connection, so a completed handshake proves it is serving.
  https_health_check {
    port = var.gateway_port
    request_path = "/healthz"
  }

  check_interval_sec  = 30
  healthy_threshold   = 2
  unhealthy_threshold = 3
}

resource "google_compute_region_backend_service" "gateway" {
  name                  = "${var.name_prefix}-gateway"
  region                = var.region
  protocol              = "TCP"
  load_balancing_scheme = "EXTERNAL"
  health_checks         = [google_compute_region_health_check.gateway.id]

  backend {
    group          = google_compute_network_endpoint_group.gateway.id
    balancing_mode = "CONNECTION"
  }
}

resource "google_compute_forwarding_rule" "gateway" {
  name                  = "${var.name_prefix}-gateway"
  region                = var.region
  ip_protocol           = "TCP"
  load_balancing_scheme = "EXTERNAL"
  ip_address            = google_compute_address.gateway.address
  port_range            = tostring(var.gateway_port)
  backend_service       = google_compute_region_backend_service.gateway.id
}

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
# instance group"). The UI backend below uses a zonal NEG, which isn't
# subject to that restriction. The gateway backend (further down) uses an
# unmanaged instance group instead of a NEG -- see the comment there -- so it
# is the one and only load-balanced instance group this VM belongs to.
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

# An unmanaged instance group, not a NEG: GCP's regional external passthrough
# NLB only supports IPv6 backends via instance groups (managed or unmanaged),
# not NEGs -- there is no GCE_VM_IP_PORT/GCE_VM_IP variant with an IPv6
# endpoint field. port_name below wires the named_port to the backend
# service, which is required once the backend is an instance group.
resource "google_compute_instance_group" "gateway" {
  name    = "${var.name_prefix}-gateway"
  zone    = var.zone
  network = var.network_self_link

  instances = [var.instance_self_link]

  named_port {
    name = "gateway"
    port = var.gateway_port
  }
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

# A second global address/forwarding-rule pair, not a change to the address
# above: a google_compute_global_address is single-stack, so IPv4 and IPv6
# each need their own reservation. Both rules target the same proxies as
# their IPv4 counterparts -- the backend service, NEG, and health check are
# unchanged, since GFE reconnects to the backend over IPv4 regardless of
# which family the client used.
resource "google_compute_global_address" "ui_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  name       = "${var.name_prefix}-ui-ipv6"
  ip_version = "IPV6"
}

resource "google_compute_global_forwarding_rule" "ui_https_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  name                  = "${var.name_prefix}-ui-https-ipv6"
  target                = google_compute_target_https_proxy.ui.id
  ip_address            = google_compute_global_address.ui_ipv6[0].address
  port_range            = "443"
  load_balancing_scheme = "EXTERNAL"
}

resource "google_compute_global_forwarding_rule" "ui_http_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  name                  = "${var.name_prefix}-ui-http-ipv6"
  target                = google_compute_target_http_proxy.ui_redirect.id
  ip_address            = google_compute_global_address.ui_ipv6[0].address
  port_range            = "80"
  load_balancing_scheme = "EXTERNAL"
}

# ---------------------------------------------------------------- gateway ----
# IPv6 is opt-in via enable_ipv6, mirroring the UI: a second reserved address
# and forwarding rule below, sharing this same backend service, health check,
# and instance group. This only works because the gateway backend is an
# instance group rather than a NEG (see google_compute_instance_group.gateway
# above) -- NEGs have no IPv6 endpoint type, but instance groups do.
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

  # Required once the backend is an instance group rather than a NEG; must
  # match the named_port on google_compute_instance_group.gateway.
  port_name = "gateway"

  backend {
    group          = google_compute_instance_group.gateway.id
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

# A second reserved address/forwarding-rule pair, not a change to the IPv4
# rule above: like the UI's global address, a regional address is
# single-stack. ipv6_endpoint_type = "NETLB" reserves the address for a
# Network Load Balancer forwarding rule rather than a VM NIC. subnetwork is
# required here (unlike the IPv4 rule) because the /96 IPv6 range is carved
# from the dual-stack subnet's external IPv6 range -- see modules/network.
resource "google_compute_address" "gateway_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  name               = "${var.name_prefix}-gateway-ipv6"
  region             = var.region
  address_type       = "EXTERNAL"
  ip_version         = "IPV6"
  ipv6_endpoint_type = "NETLB"
}

resource "google_compute_forwarding_rule" "gateway_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  name                  = "${var.name_prefix}-gateway-ipv6"
  region                = var.region
  ip_protocol           = "TCP"
  ip_version            = "IPV6"
  ip_address            = google_compute_address.gateway_ipv6[0].address
  subnetwork            = var.subnetwork_self_link
  load_balancing_scheme = "EXTERNAL"
  port_range            = tostring(var.gateway_port)
  backend_service       = google_compute_region_backend_service.gateway.id
}

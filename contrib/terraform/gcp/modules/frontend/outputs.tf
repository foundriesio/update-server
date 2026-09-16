# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "ui_ip" {
  description = "Reserved global IP for the UI load balancer, for the UI's A record."
  value       = google_compute_global_address.ui.address
}

output "ui_ipv6" {
  description = "Reserved global IPv6 for the UI load balancer, for the UI's AAAA record. \"\" when enable_ipv6 is false."
  value       = var.enable_ipv6 ? google_compute_global_address.ui_ipv6[0].address : ""
}

output "gateway_ip" {
  description = "Reserved regional IP for the gateway load balancer, for the gateway's A record."
  value       = google_compute_address.gateway.address
}

output "gateway_ipv6" {
  description = "Reserved regional IPv6 for the gateway load balancer, for the gateway's AAAA record. \"\" when enable_ipv6 is false."
  value       = var.enable_ipv6 ? google_compute_address.gateway_ipv6[0].address : ""
}

output "certificate_id" {
  description = "Self-link of the UI's Google-managed SSL certificate."
  value       = google_compute_managed_ssl_certificate.ui.id
}

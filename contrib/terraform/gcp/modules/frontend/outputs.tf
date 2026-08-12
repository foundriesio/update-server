# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "ui_ip" {
  description = "Reserved global IP for the UI load balancer, for the UI's A record."
  value       = google_compute_global_address.ui.address
}

output "gateway_ip" {
  description = "Reserved regional IP for the gateway load balancer, for the gateway's A record."
  value       = google_compute_address.gateway.address
}

output "certificate_id" {
  description = "Self-link of the UI's Google-managed SSL certificate."
  value       = google_compute_managed_ssl_certificate.ui.id
}

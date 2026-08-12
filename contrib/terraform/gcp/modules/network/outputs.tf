# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "network_self_link" {
  description = "Self-link of the VPC network."
  value       = google_compute_network.main.self_link
}

output "subnetwork_self_link" {
  description = "Self-link of the public subnet."
  value       = google_compute_subnetwork.public.self_link
}

output "server_tag" {
  description = "Network tag applied to the update server instance and matched by the firewall rules above."
  value       = local.server_tag
}

# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "instance_id" {
  description = "ID of the update server instance."
  value       = google_compute_instance.server.instance_id
}

output "instance_name" {
  description = "Name of the update server instance, for `gcloud compute ssh`."
  value       = google_compute_instance.server.name
}

output "instance_self_link" {
  description = "Self-link of the instance, used as the instance group target."
  value       = google_compute_instance.server.self_link
}

output "internal_ip" {
  description = "Internal IP, used as the load balancer health check target."
  value       = google_compute_instance.server.network_interface[0].network_ip
}

output "public_ip" {
  description = "Public address: static when assign_static_ip is set, ephemeral otherwise."
  value       = google_compute_instance.server.network_interface[0].access_config[0].nat_ip
}

output "data_disk_id" {
  description = "ID of the persistent data disk."
  value       = google_compute_disk.data.id
}

output "secret_prefix" {
  description = "Secret Manager name prefix for this deployment."
  value       = local.secret_prefix
}

output "service_account_email" {
  description = "Email of the instance's service account."
  value       = google_service_account.server.email
}

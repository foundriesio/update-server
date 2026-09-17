# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "ui_url" {
  description = "Web UI address, served by Caddy with a Let's Encrypt certificate."
  value       = "https://${var.hostname}"
}

output "device_gateway_url" {
  description = "Gateway address devices connect to, exposed directly."
  value       = "https://${var.hostname}:${var.gateway_port}"
}

output "public_ip" {
  description = "Static IP. Point var.hostname here if managing DNS yourself."
  value       = module.server.public_ip
}

output "instance_id" {
  description = "Instance ID."
  value       = module.server.instance_id
}

output "instance_name" {
  description = "Instance name, for `gcloud compute ssh <name> --tunnel-through-iap`."
  value       = module.server.instance_name
}

output "data_disk_id" {
  description = "Persistent data disk ID."
  value       = module.server.data_disk_id
}

output "secret_prefix" {
  description = "Secret Manager prefix holding the escrowed keys."
  value       = module.server.secret_prefix
}

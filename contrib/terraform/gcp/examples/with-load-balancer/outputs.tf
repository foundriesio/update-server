# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "ui_url" {
  description = "Web UI address."
  value       = "https://${var.hostname}"
}

output "device_gateway_url" {
  description = "Gateway address devices connect to."
  value       = "https://${var.gateway_hostname}:${var.gateway_port}"
}

output "ui_ip" {
  description = "Global LB IP. Point var.hostname here if managing DNS yourself."
  value       = module.frontend.ui_ip
}

output "gateway_ip" {
  description = "Regional LB IP. Point var.gateway_hostname here if managing DNS yourself."
  value       = module.frontend.gateway_ip
}

output "ui_ipv6" {
  description = "Global LB IPv6. \"\" when enable_ipv6 is false."
  value       = module.frontend.ui_ipv6
}

output "gateway_ipv6" {
  description = "Regional LB IPv6. \"\" when enable_ipv6 is false."
  value       = module.frontend.gateway_ipv6
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

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
  description = "Elastic IP. Point var.hostname here if managing DNS yourself."
  value       = module.server.public_ip
}

output "ipv6_address" {
  description = "Public IPv6 address, or null when enable_ipv6 is false. Point an AAAA record here if managing DNS yourself."
  value       = module.server.ipv6_address
}

output "instance_id" {
  description = "Instance ID, for `aws ssm start-session --target <id>`."
  value       = module.server.instance_id
}

output "data_volume_id" {
  description = "Persistent data volume ID."
  value       = module.server.data_volume_id
}

output "secret_prefix" {
  description = "Secrets Manager prefix holding the escrowed keys."
  value       = module.server.secret_prefix
}

output "cloudwatch_log_group_name" {
  description = "CloudWatch Logs group receiving the fioserver journal, or null when disabled."
  value       = module.server.cloudwatch_log_group_name
}

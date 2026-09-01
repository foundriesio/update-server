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

output "alb_dns_name" {
  description = "ALB DNS name. Point var.hostname here if managing DNS yourself."
  value       = module.frontend.alb_dns_name
}

output "nlb_dns_name" {
  description = "NLB DNS name. Point var.gateway_hostname here if managing DNS yourself."
  value       = module.frontend.nlb_dns_name
}

output "acm_validation_records" {
  description = "Create these if hosted_zone_id is empty; apply blocks until they resolve."
  value       = module.dns.validation_records
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

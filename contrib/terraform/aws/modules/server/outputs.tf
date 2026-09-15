# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "instance_id" {
  description = "ID of the update server instance."
  value       = aws_instance.server.id
}

output "private_ip" {
  description = "Private IP, used as the load balancer target."
  value       = aws_instance.server.private_ip
}

output "public_ip" {
  description = "Public address: the Elastic IP when one is assigned."
  value       = var.assign_eip ? aws_eip.server[0].public_ip : aws_instance.server.public_ip
}

output "ipv6_address" {
  description = "Public IPv6 address, or null when enable_ipv6 is false. Elastic IPs are IPv4-only, so this is the instance's own address, not an EIP."
  value       = var.enable_ipv6 ? try(aws_instance.server.ipv6_addresses[0], null) : null
}

output "data_volume_id" {
  description = "ID of the persistent data volume."
  value       = aws_ebs_volume.data.id
}

output "secret_prefix" {
  description = "Secrets Manager name prefix for this deployment."
  value       = local.secret_prefix
}

output "secret_arns" {
  description = "ARNs of every secret this deployment reads from Secrets Manager."
  value = merge(
    { auth-config = data.aws_secretsmanager_secret.auth_config.arn },
    { for k, s in data.aws_secretsmanager_secret.escrow : k => s.arn },
  )
}

output "iam_role_name" {
  description = "Name of the instance's IAM role."
  value       = aws_iam_role.server.name
}

output "cloudwatch_log_group_name" {
  description = "CloudWatch Logs group receiving the fioserver journal, or null when enable_cloudwatch_logs is false."
  value       = var.enable_cloudwatch_logs ? aws_cloudwatch_log_group.fioserver[0].name : null
}

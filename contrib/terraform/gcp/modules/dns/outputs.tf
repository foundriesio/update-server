# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

output "managing_dns" {
  description = "Whether this module is creating Cloud DNS records."
  value       = local.manage_dns
}

output "records" {
  description = <<-EOT
    DNS name to IP address mapping this module manages, or would manage if
    managed_zone_name were set. When managing_dns is false, create these by
    hand before the UI's managed certificate can issue.
  EOT
  value       = local.records
}

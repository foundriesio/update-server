# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Builds the AMI that both Terraform examples deploy: Debian 13 plus the
# fioserver release binary, the systemd units, and a read-only root filesystem.

packer {
  required_version = ">= 1.9.0"
  required_plugins {
    amazon = {
      version = ">= 1.3.0"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

variable "region" {
  type        = string
  description = "AWS region to build the AMI in."
  default     = "us-east-1"
}

variable "fioserver_version" {
  type        = string
  description = <<-EOT
    Release tag to install, e.g. "v0.9.2". Required and deliberately not
    defaulted to "latest": pinning it is what makes an AMI reproducible.
  EOT
}

variable "instance_type" {
  type        = string
  description = "Instance type used for the build itself (not the deployment)."
  default     = ""
}

variable "ami_name_prefix" {
  type        = string
  description = "Prefix for the resulting AMI name."
  default     = "fioserver"
}

variable "ami_users" {
  type        = list(string)
  description = "Extra AWS account IDs to share the resulting AMI with."
  default     = []
}

locals {
  debian_owner  = "136693071363"
  build_type    = var.instance_type != "" ? var.instance_type : "t3.small"
  ssh_user      = "admin"
  ami_timestamp = formatdate("YYYYMMDD-hhmmss", timestamp())
}

source "amazon-ebs" "fioserver" {
  region        = var.region
  instance_type = local.build_type
  ssh_username  = local.ssh_user

  source_ami_filter {
    filters = {
      name                = "debian-13-amd64-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    owners      = [local.debian_owner]
    most_recent = true
  }

  ami_name        = "${var.ami_name_prefix}-${var.fioserver_version}-${local.ami_timestamp}"
  ami_description = "Foundries update server ${var.fioserver_version}, read-only root"
  ami_users       = var.ami_users

  # The root volume only holds the OS; all state lives on the separate data
  # volume Terraform attaches.
  launch_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = 8
    volume_type           = "gp3"
    delete_on_termination = true
  }

  # Require IMDSv2 during the build too, matching the deployed instance.
  imds_support = "v2.0"

  tags = {
    Name              = "${var.ami_name_prefix}-${var.fioserver_version}"
    fioserver_version = var.fioserver_version
    source_ami        = "{{ .SourceAMI }}"
    source_ami_name   = "{{ .SourceAMIName }}"
    built_by          = "packer"
  }
}

build {
  name    = "fioserver"
  sources = ["source.amazon-ebs.fioserver"]

  provisioner "file" {
    source      = "${path.root}/files"
    destination = "/tmp/files"
  }

  provisioner "shell" {
    execute_command = "chmod +x {{ .Path }}; sudo {{ .Path }} ${var.fioserver_version}"
    script          = "${path.root}/files/provision.sh"
  }

  post-processor "manifest" {
    output     = "${path.root}/manifest.json"
    strip_path = true
  }
}

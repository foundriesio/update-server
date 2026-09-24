# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Builds the image that the Terraform example deploys: Debian 13 plus the
# fioserver release binary, the systemd units, and a read-only root filesystem.

packer {
  required_version = ">= 1.9.0"
  required_plugins {
    googlecompute = {
      version = ">= 1.1.0"
      source  = "github.com/hashicorp/googlecompute"
    }
  }
}

variable "project_id" {
  type        = string
  description = "GCP project to build the image in."
}

variable "zone" {
  type        = string
  description = "GCP zone to build the image in."
  default     = "us-central1-a"
}

variable "fioserver_version" {
  type        = string
  description = <<-EOT
    Release tag to install, e.g. "v0.9.2". Required and deliberately not
    defaulted to "latest": pinning it is what makes an image reproducible.
  EOT
}

variable "architecture" {
  type        = string
  description = "Target architecture: amd64 or arm64."
  default     = "amd64"

  validation {
    condition     = contains(["amd64", "arm64"], var.architecture)
    error_message = "The architecture must be amd64 or arm64."
  }
}

variable "machine_type" {
  type        = string
  description = "Machine type used for the build itself (not the deployment)."
  default     = ""
}

variable "image_name_prefix" {
  type        = string
  description = "Prefix for the resulting image name."
  default     = "fioserver"
}

locals {
  # Google publishes separate image families per architecture, unlike AWS's
  # single filtered AMI search.
  source_family = var.architecture == "arm64" ? "debian-13-arm64" : "debian-13"
  build_type    = var.machine_type != "" ? var.machine_type : (var.architecture == "arm64" ? "t2a-standard-1" : "e2-small")
  # Image names/labels are restricted to lowercase letters, digits, and
  # dashes -- no dots -- so the "v0.9.2"-style release tag has to be
  # sanitized before it can appear in either.
  version_slug    = replace(var.fioserver_version, ".", "-")
  image_timestamp = formatdate("YYYYMMDD-hhmmss", timestamp())
}

source "googlecompute" "fioserver" {
  project_id   = var.project_id
  zone         = var.zone
  machine_type = local.build_type

  source_image_family     = local.source_family
  source_image_project_id = ["debian-cloud"]

  disk_size = 10
  disk_type = "pd-balanced"

  image_name        = "${var.image_name_prefix}-${local.version_slug}-${var.architecture}-${local.image_timestamp}"
  image_description = "Foundries update server ${var.fioserver_version} (${var.architecture}), read-only root"

  image_labels = {
    fioserver_version = local.version_slug
    architecture      = var.architecture
    built_by          = "packer"
  }

  # The Debian cloud image's guest agent provisions this account from the
  # temporary SSH key Packer injects via instance metadata -- there is no
  # baked-in user the way AWS's Debian AMI ships "admin".
  ssh_username = "packer"

  # Matches the deployed instance's shielded-VM settings (see modules/server).
  enable_secure_boot          = true
  enable_vtpm                 = true
  enable_integrity_monitoring = true
}

build {
  name    = "fioserver"
  sources = ["source.googlecompute.fioserver"]

  provisioner "file" {
    source      = "${path.root}/files"
    destination = "/tmp/files"
  }

  provisioner "shell" {
    execute_command = "chmod +x {{ .Path }}; sudo {{ .Path }} ${var.fioserver_version} ${var.architecture}"
    script          = "${path.root}/files/provision.sh"
  }

  post-processor "manifest" {
    output     = "${path.root}/manifest.json"
    strip_path = true
  }
}

# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# The instance, its persistent data volume, the secret escrow, and backups.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }
}

data "aws_region" "current" {}

locals {
  tags = merge(var.tags, { Name = var.name_prefix })

  secret_prefix = "${var.name_prefix}/${var.hostname}"

  # Reuses the secret_prefix naming convention rather than inventing a new one.
  log_group_name = "/${local.secret_prefix}"

  # Written by scripts/init-secrets.sh, never by Terraform or the instance.
  # Declared here so the IAM policy can name them explicitly rather than
  # using a wildcard.
  escrowed_secrets = ["hmac-secret", "certs-archive", "tuf-archive"]

  # With an ALB in front the instance must accept connections over the network;
  # with Caddy on the box, loopback is enough and keeps 8080 unreachable.
  ui_addr = var.enable_caddy ? "127.0.0.1:8080" : "0.0.0.0:8080"
}

# ------------------------------------------------------------ data volume ----
# A standalone volume, deliberately not a root block device, so the server's
# state survives replacing the instance. There is no prevent_destroy: it would
# make `terraform destroy` fail and strand the volume. The DLM snapshots and the
# secret escrow are the recovery path -- see the README.
resource "aws_ebs_volume" "data" {
  availability_zone = var.availability_zone
  size              = var.data_volume_size
  type              = "gp3"
  encrypted         = true

  tags = merge(local.tags, {
    Name = "${var.name_prefix}-data"
    # The DLM policy selects the volume by this tag.
    fioserver-backup = var.name_prefix
  })
}

resource "aws_volume_attachment" "data" {
  # Requested as /dev/sdf, but Nitro instances expose it as /dev/nvme1n1. The
  # instance never refers to either: it mounts by filesystem label.
  device_name = "/dev/sdf"
  volume_id   = aws_ebs_volume.data.id
  instance_id = aws_instance.server.id

  # Do not force-detach a mounted filesystem when replacing the instance.
  skip_destroy = true
}

# ---------------------------------------------------------------- secrets ----
# Terraform never creates these -- scripts/init-secrets.sh does, before
# `apply` runs. Looking them up with a data source (rather than owning them
# as a resource) means a deployment that skips the script fails fast here,
# at plan/apply time, instead of succeeding and leaving the instance to fail
# loudly at boot.
data "aws_secretsmanager_secret" "auth_config" {
  name = "${local.secret_prefix}/auth-config"
}

data "aws_secretsmanager_secret" "escrow" {
  for_each = toset(local.escrowed_secrets)

  name = "${local.secret_prefix}/${each.key}"
}

# -------------------------------------------------------------------- IAM ----
data "aws_iam_policy_document" "assume_ec2" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "server" {
  name_prefix        = "${var.name_prefix}-server-"
  assume_role_policy = data.aws_iam_policy_document.assume_ec2.json
  tags               = local.tags
}

data "aws_iam_policy_document" "server" {
  # Scoped to exactly the five secrets this deployment owns, not a prefix
  # wildcard. Read-only: the instance only ever restores from escrow (see
  # fioserver-bootstrap.sh's State B); Escrow is populated by init-secrets.sh.
  statement {
    sid = "SecretEscrow"
    actions = [
      "secretsmanager:GetSecretValue",
      "secretsmanager:DescribeSecret",
    ]
    resources = concat(
      [data.aws_secretsmanager_secret.auth_config.arn],
      [for s in data.aws_secretsmanager_secret.escrow : s.arn],
    )
  }

  statement {
    sid       = "DescribeVolumes"
    actions   = ["ec2:DescribeVolumes", "ec2:DescribeTags"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "server" {
  name_prefix = "${var.name_prefix}-server-"
  role        = aws_iam_role.server.id
  policy      = data.aws_iam_policy_document.server.json
}

# Session Manager access, so the read-only root and the shared SSH host keys do
# not force port 22 open.
resource "aws_iam_role_policy_attachment" "ssm" {
  count = var.enable_ssm ? 1 : 0

  role       = aws_iam_role.server.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# ------------------------------------------------------------- CloudWatch ----
# Terraform owns the log group (retention, tags, destroy) rather than letting
# the CloudWatch agent auto-create it.
resource "aws_cloudwatch_log_group" "fioserver" {
  count = var.enable_cloudwatch_logs ? 1 : 0

  name              = local.log_group_name
  retention_in_days = var.cloudwatch_log_retention_days
  tags              = local.tags
}

data "aws_iam_policy_document" "cloudwatch_logs" {
  count = var.enable_cloudwatch_logs ? 1 : 0

  statement {
    sid = "FioserverLogs"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
      "logs:DescribeLogStreams",
    ]
    resources = ["${aws_cloudwatch_log_group.fioserver[0].arn}:*"]
  }
}

resource "aws_iam_role_policy" "cloudwatch_logs" {
  count = var.enable_cloudwatch_logs ? 1 : 0

  name_prefix = "${var.name_prefix}-cwlogs-"
  role        = aws_iam_role.server.id
  policy      = data.aws_iam_policy_document.cloudwatch_logs[0].json
}

resource "aws_iam_instance_profile" "server" {
  name_prefix = "${var.name_prefix}-server-"
  role        = aws_iam_role.server.name
  tags        = local.tags
}

# --------------------------------------------------------------- instance ----
resource "aws_instance" "server" {
  ami                    = var.ami_id
  instance_type          = var.instance_type
  subnet_id              = var.subnet_id
  vpc_security_group_ids = [var.security_group_id]
  iam_instance_profile   = aws_iam_instance_profile.server.name
  key_name               = var.ssh_key_name == "" ? null : var.ssh_key_name
  ipv6_address_count     = var.enable_ipv6 ? 1 : 0

  # Configuration only -- never secrets. The instance fetches those from Secrets
  # Manager using its instance profile, so nothing sensitive lands in user_data
  # (which is readable via IMDS) or in Terraform state.
  user_data = <<-EOT
    #cloud-config
    write_files:
      - path: /etc/fioserver/env
        permissions: '0644'
        content: |
          FIOSERVER_HOSTNAME=${var.hostname}
          FIOSERVER_SECRET_PREFIX=${local.secret_prefix}
          FIOSERVER_UI_ADDR=${local.ui_addr}
          FIOSERVER_GATEWAY_ADDR=0.0.0.0:${var.gateway_port}
          AWS_REGION=${data.aws_region.current.name}
%{if var.enable_cloudwatch_logs~}
      - path: /etc/fioserver/cloudwatch-agent.json
        permissions: '0644'
        content: |
          {
            "logs": {
              "logs_collected": {
                "journald": {
                  "collect_list": [
                    {
                      "log_group_name": "${local.log_group_name}",
                      "log_stream_name": "fioserver",
                      "units": ["fioserver"]
                    }
                  ]
                }
              }
            }
          }
%{endif~}
    runcmd:
      - [systemctl, enable, --now, fioserver-volume-init.service]
      - [systemctl, enable, --now, data.mount]
      - [systemctl, enable, --now, fioserver-bootstrap.service]
      - [systemctl, enable, --now, fioserver.service]
%{if var.enable_caddy~}
      - [systemctl, enable, --now, caddy.service]
%{endif~}
%{if var.enable_cloudwatch_logs~}
      - [/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl, -a, fetch-config, -m, ec2, -s, -c, "file:/etc/fioserver/cloudwatch-agent.json"]
%{endif~}
  EOT

  # A config change should not silently destroy the instance and its state.
  user_data_replace_on_change = false

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  root_block_device {
    volume_size           = var.root_volume_size
    volume_type           = "gp3"
    encrypted             = true
    delete_on_termination = true
  }

  tags = merge(local.tags, { Name = "${var.name_prefix}-server" })

  lifecycle {
    # Rebuilding the AMI is the OS patching path; adopt a new one only when the
    # operator explicitly replaces the instance.
    ignore_changes = [ami]
  }
}

resource "aws_eip" "server" {
  count = var.assign_eip ? 1 : 0

  instance = aws_instance.server.id
  domain   = "vpc"

  tags = merge(local.tags, { Name = "${var.name_prefix}-eip" })
}

# --------------------------------------------------------------- backups ----
# DLM needs a service role. The console creates AWSDataLifecycleManagerDefaultRole
# implicitly, but the API never does -- so on a fresh account relying on it makes
# apply fail. Create our own.
data "aws_iam_policy_document" "assume_dlm" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["dlm.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dlm" {
  name_prefix        = "${var.name_prefix}-dlm-"
  assume_role_policy = data.aws_iam_policy_document.assume_dlm.json
  tags               = local.tags
}

resource "aws_iam_role_policy_attachment" "dlm" {
  role       = aws_iam_role.dlm.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSDataLifecycleManagerServiceRole"
}

resource "aws_dlm_lifecycle_policy" "data" {
  description        = "Daily snapshots of the ${var.name_prefix} data volume"
  execution_role_arn = aws_iam_role.dlm.arn
  state              = "ENABLED"
  tags               = local.tags

  policy_details {
    resource_types = ["VOLUME"]

    target_tags = {
      fioserver-backup = var.name_prefix
    }

    schedule {
      name = "daily"

      create_rule {
        interval      = 24
        interval_unit = "HOURS"
        times         = [var.snapshot_start_time]
      }

      retain_rule {
        interval      = var.snapshot_retention_days
        interval_unit = "DAYS"
      }

      # Note: these snapshots are crash-consistent. SQLite in WAL mode recovers
      # from them in practice, but see the README for the caveat.
      copy_tags = true
    }
  }
}

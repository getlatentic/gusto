# What outlives a demo host: the role and profile every host has, the security group, the
# launch template a host is made from, and the role GitHub Actions takes to launch and remove
# hosts. Nothing here costs while no host is up. A host is made from the launch template and
# removes itself, so no host is in this state.
#
# The CI role can launch and remove hosts tagged project=gusto-poc and put objects in the
# bucket. It has no IAM permission, so it cannot change what it may do: this stack is applied by
# a person, with bootstrap.sh, and changes to it are rare.

data "aws_caller_identity" "here" {}

data "aws_partition" "here" {}

data "aws_vpc" "default" {
  default = true
}

locals {
  account  = data.aws_caller_identity.here.account_id
  host     = "gusto-poc-demo-host"
  ci       = "gusto-poc-ci"
  bucket   = "arn:aws:s3:::${var.bucket}"
  ssm_path = "arn:aws:ssm:${var.region}:${local.account}:parameter/gusto/poc/*"
}

# ── The host ─────────────────────────────────────────────────────────────────

resource "aws_security_group" "host" {
  name        = local.host
  description = "A demo host: https from anywhere and nothing else"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "https for the public cloud"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "the hosts of one demo reach each other: the front door reaches the agent of a cell and the servers on it"
    from_port   = 0
    to_port     = 65535
    protocol    = "tcp"
    self        = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # The hosts and the file system's mount targets use the group until they are moved to its successor, so a group
  # whose name changes is made before the old one is removed.
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_iam_role" "host" {
  name = local.host
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

# Session Manager, which is how a person reaches a host and how the CI role tells one to power off.
resource "aws_iam_role_policy_attachment" "host_ssm" {
  role       = aws_iam_role.host.name
  policy_arn = "arn:${data.aws_partition.here.partition}:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy" "host" {
  name = "${local.host}-reads-and-keeps"
  role = aws_iam_role.host.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["ssm:GetParameter", "ssm:GetParameters"], Resource = local.ssm_path },
      { Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject"], Resource = "${local.bucket}/*" },
      { Effect = "Allow", Action = ["s3:ListBucket"], Resource = local.bucket },
    ]
  })
}

resource "aws_iam_instance_profile" "host" {
  name = local.host
  role = aws_iam_role.host.name
}

data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}

resource "aws_launch_template" "host" {
  name                                 = local.host
  image_id                             = data.aws_ssm_parameter.ubuntu.value
  instance_type                        = var.instance_type
  instance_initiated_shutdown_behavior = "terminate"
  vpc_security_group_ids               = [aws_security_group.host.id]

  iam_instance_profile {
    name = aws_iam_instance_profile.host.name
  }

  cpu_options {
    nested_virtualization = "enabled"
  }

  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  block_device_mappings {
    device_name = "/dev/sda1"
    ebs {
      volume_size           = 40
      volume_type           = "gp3"
      delete_on_termination = true
    }
  }

  tag_specifications {
    resource_type = "instance"
    tags          = { project = "gusto-poc", name = "demo-host" }
  }

  tag_specifications {
    resource_type = "volume"
    tags          = { project = "gusto-poc", name = "demo-host" }
  }
}

# The front door and the cells of a demo, when it has more than one host. Each is made from a template of
# its own: a front door runs no computer, so it needs no nested virtualization and a small type will do.
locals {
  hosts = {
    orchestrator = { type = var.orchestrator_instance_type, nested = false, project = "gusto-poc" }
    cell         = { type = var.cell_instance_type, nested = true, project = "gusto-poc" }
    # The cells the live front door starts. They are tagged for it alone, so the workflows that remove a demo's
    # hosts never reach them, and it never reaches theirs.
    cell-live = { type = var.cell_live_instance_type, nested = true, project = "gusto-live" }
  }
}

# The image a cell of the live front door is started from. It begins as Ubuntu's, so a cell starts as it always did, and
# bake-cell-image.sh puts the image it makes here, which is read each time a host is started and so needs no change to this stack.
resource "aws_ssm_parameter" "cell_image" {
  name        = "/gusto/poc/cell-ami"
  description = "The image a cell of the live front door is started from, made by bake-cell-image.sh"
  type        = "String"
  data_type   = "aws:ec2:image"
  value       = data.aws_ssm_parameter.ubuntu.value

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_launch_template" "role" {
  for_each                             = local.hosts
  name                                 = "${local.host}-${each.key}"
  image_id                             = each.key == "cell-live" ? "resolve:ssm:${aws_ssm_parameter.cell_image.name}" : data.aws_ssm_parameter.ubuntu.value
  instance_type                        = each.value.type
  instance_initiated_shutdown_behavior = "terminate"
  vpc_security_group_ids               = [aws_security_group.host.id]

  iam_instance_profile {
    name = aws_iam_instance_profile.host.name
  }

  dynamic "cpu_options" {
    for_each = each.value.nested ? [1] : []
    content {
      nested_virtualization = "enabled"
    }
  }

  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  block_device_mappings {
    device_name = "/dev/sda1"
    ebs {
      volume_size           = 40
      volume_type           = "gp3"
      delete_on_termination = true
      # A disk made from the image of a cell is read from its snapshot a block at a time, and is read whole to start the first sandbox. A
      # host of a cell is given the throughput to be read in many streams, which is about five cents an hour while it runs.
      iops       = each.key == "cell-live" ? 6000 : null
      throughput = each.key == "cell-live" ? 500 : null
    }
  }

  tag_specifications {
    resource_type = "instance"
    tags          = { project = each.value.project, name = "demo-${each.key}" }
  }

  tag_specifications {
    resource_type = "volume"
    tags          = { project = each.value.project, name = "demo-${each.key}" }
  }
}

# ── The front door that stays up ─────────────────────────────────────────────

# It runs the public demo: nginx and the certificate, the front door, and the scaler that starts a host for a cell when
# an organization needs one. Its role is its own and is the only one that may start and stop hosts, which it may only
# when they are tagged for it, so a cell, which runs other people's code, can start nothing.
locals {
  live      = "gusto-poc-front-door"
  live_tag  = "gusto-live"
  ec2_arn   = "arn:aws:ec2:${var.region}:${local.account}"
  cell_live = aws_launch_template.role["cell-live"]
}

resource "aws_iam_role" "live" {
  name = local.live
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy_attachment" "live_ssm" {
  role       = aws_iam_role.live.name
  policy_arn = "arn:${data.aws_partition.here.partition}:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy" "live" {
  name = "${local.live}-reads-keeps-and-scales-its-cells"
  role = aws_iam_role.live.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["ssm:GetParameter", "ssm:GetParameters"], Resource = local.ssm_path },
      { Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject"], Resource = "${local.bucket}/*" },
      { Effect = "Allow", Action = ["s3:ListBucket"], Resource = local.bucket },
      {
        Sid       = "StartCellsTaggedForTheLiveFrontDoor"
        Effect    = "Allow"
        Action    = "ec2:RunInstances"
        Resource  = ["${local.ec2_arn}:instance/*", "${local.ec2_arn}:volume/*"]
        Condition = { StringEquals = { "aws:RequestTag/project" = local.live_tag } }
      },
      {
        Sid    = "FromTheLiveCellTemplateOnly"
        Effect = "Allow"
        Action = "ec2:RunInstances"
        Resource = [
          "${local.ec2_arn}:launch-template/${local.cell_live.id}",
          "${local.ec2_arn}:subnet/*",
          "${local.ec2_arn}:network-interface/*",
          "${local.ec2_arn}:security-group/${aws_security_group.host.id}",
          "arn:aws:ec2:${var.region}::image/*",
          "${local.ec2_arn}:image/*",
          "arn:aws:ec2:${var.region}::snapshot/*",
          "${local.ec2_arn}:snapshot/*",
        ]
      },
      { Sid = "Tag", Effect = "Allow", Action = "ec2:CreateTags", Resource = "*", Condition = { StringEquals = { "ec2:CreateAction" = "RunInstances" } } },
      { Sid = "GiveACellItsRole", Effect = "Allow", Action = "iam:PassRole", Resource = aws_iam_role.host.arn, Condition = { StringEquals = { "iam:PassedToService" = "ec2.amazonaws.com" } } },
      { Sid = "Look", Effect = "Allow", Action = ["ec2:DescribeInstances", "ec2:DescribeInstanceStatus", "ec2:DescribeVolumes"], Resource = "*" },
      {
        Sid       = "StopOnlyWhatItStarted"
        Effect    = "Allow"
        Action    = "ec2:TerminateInstances"
        Resource  = "${local.ec2_arn}:instance/*"
        Condition = { StringEquals = { "ec2:ResourceTag/project" = local.live_tag, "ec2:ResourceTag/scaler" = "on" } }
      },
      {
        # A kept cell is stopped when idle, and given today's start-up script before it is started again.
        Sid       = "KeepOnlyWhatItStarted"
        Effect    = "Allow"
        Action    = ["ec2:StartInstances", "ec2:StopInstances", "ec2:ModifyInstanceAttribute"]
        Resource  = "${local.ec2_arn}:instance/*"
        Condition = { StringEquals = { "ec2:ResourceTag/project" = local.live_tag, "ec2:ResourceTag/scaler" = "on" } }
      },
      {
        # A kept cell's root disk is taken down to the baseline throughput when it is first stopped.
        Sid       = "LowerOnlyTheDisksOfItsCells"
        Effect    = "Allow"
        Action    = "ec2:ModifyVolume"
        Resource  = "${local.ec2_arn}:volume/*"
        Condition = { StringEquals = { "ec2:ResourceTag/project" = local.live_tag } }
      },
    ]
  })
}

resource "aws_iam_instance_profile" "live" {
  name = local.live
  role = aws_iam_role.live.name
}

resource "aws_eip" "live" {
  domain = "vpc"
  tags   = { project = local.live_tag, name = "front-door" }
}

# It is started from the bundle like any host and keeps up to date by running the script again (gusto-upgrade). Its
# disk holds the accounts database and the task index (cloud.db and index/ under /var/lib/gusto) and everything else
# is on the file system or in the bucket, so replacing it takes a copy of those two. It is stopped, not removed, if it
# is powered off.
resource "aws_instance" "live" {
  ami                                  = data.aws_ssm_parameter.ubuntu.value
  instance_type                        = var.front_door_instance_type
  iam_instance_profile                 = aws_iam_instance_profile.live.name
  vpc_security_group_ids               = [aws_security_group.host.id]
  instance_initiated_shutdown_behavior = "stop"

  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  root_block_device {
    volume_size = 10
    volume_type = "gp3"
  }

  # The start-up script reads what it is to be from its environment, which is set here. It runs once, on the first
  # start, and what it was started with is kept on the host for the upgrades after.
  user_data = join("\n", [
    "#!/bin/bash",
    "export RELEASE=latest BUCKET=${var.bucket} REGION=${var.region} ROLE=orchestrator RUN=live EFS=${aws_efs_file_system.shared.id} LIVE=on LEASE_MINUTES=30 MAX_HOURS=24",
    file("${path.module}/user-data.sh"),
  ])
  tags = { project = local.live_tag, name = "front-door" }

  lifecycle {
    ignore_changes = [ami, user_data]
  }
}

resource "aws_eip_association" "live" {
  instance_id   = aws_instance.live.id
  allocation_id = aws_eip.live.id
}

# ── The file system the hosts of a demo share ────────────────────────────────

# The organizations' data, which the front door and a cell both read and write. It is not in a
# host's disk, since a cell is a host that goes, and each demo has a directory of it that the next
# one removes. It costs 30 cents a gigabyte-month, and a demo holds a few megabytes.
resource "aws_efs_file_system" "shared" {
  encrypted = true
  tags      = { project = "gusto-poc", Name = "${local.host}-shared" }
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_security_group" "efs" {
  name        = "${local.host}-shared"
  description = "NFS from the demo hosts"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description     = "NFS from a demo host"
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [aws_security_group.host.id]
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_efs_mount_target" "shared" {
  for_each        = toset(data.aws_subnets.default.ids)
  file_system_id  = aws_efs_file_system.shared.id
  subnet_id       = each.value
  security_groups = [aws_security_group.efs.id]
}

# ── GitHub Actions ───────────────────────────────────────────────────────────

# GitHub's identity provider is one for the whole account. Whoever applies this stack with the rights to
# make one lets this stack make it (manage_github_provider); otherwise it is made once by hand, which
# README.md says how. IAM does not check that the provider a trust policy names is there, so a role
# made without one exists and cannot be assumed.
resource "aws_iam_openid_connect_provider" "github" {
  count          = var.manage_github_provider ? 1 : 0
  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
}

locals {
  github_provider = "arn:${data.aws_partition.here.partition}:iam::${local.account}:oidc-provider/token.actions.githubusercontent.com"
}

resource "aws_iam_role" "ci" {
  depends_on           = [aws_iam_openid_connect_provider.github]
  name                 = local.ci
  max_session_duration = 3600
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = local.github_provider }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
          "token.actions.githubusercontent.com:sub" = "${var.github_subject}:ref:refs/heads/main"
        }
      }
    }]
  })
}

# Making a cell's image when what it is made from changes, as bake-cell-image.sh does: a host from the live cell template, an image of
# it once it is stopped, the parameter the template reads pointed at the image, and the image before last removed. Each action is
# held to what is tagged for the project, and the parameter to the one the cell template reads.
resource "aws_iam_role_policy" "ci_bake" {
  name = "${local.ci}-makes-the-cell-image"
  role = aws_iam_role.ci.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "StartABakeHostFromTheLiveCellTemplate"
        Effect = "Allow"
        Action = "ec2:RunInstances"
        Resource = [
          "arn:aws:ec2:${var.region}:${local.account}:launch-template/${local.cell_live.id}",
          "arn:aws:ec2:${var.region}:${local.account}:snapshot/*",
          "arn:aws:ec2:${var.region}::snapshot/*",
        ]
      },
      {
        Sid       = "StopAndImageOnlyTaggedHosts"
        Effect    = "Allow"
        Action    = ["ec2:StopInstances", "ec2:CreateImage"]
        Resource  = "arn:aws:ec2:${var.region}:${local.account}:instance/*"
        Condition = { StringEquals = { "ec2:ResourceTag/project" = "gusto-poc" } }
      },
      {
        Sid      = "TheImageAndItsSnapshot"
        Effect   = "Allow"
        Action   = "ec2:CreateImage"
        Resource = ["arn:aws:ec2:${var.region}::image/*", "arn:aws:ec2:${var.region}::snapshot/*"]
      },
      { Sid = "TagTheImage", Effect = "Allow", Action = "ec2:CreateTags", Resource = "*", Condition = { StringEquals = { "ec2:CreateAction" = "CreateImage" } } },
      {
        Sid       = "RemoveOnlyTaggedImages"
        Effect    = "Allow"
        Action    = ["ec2:DeregisterImage", "ec2:DeleteSnapshot"]
        Resource  = ["arn:aws:ec2:${var.region}::image/*", "arn:aws:ec2:${var.region}::snapshot/*"]
        Condition = { StringEquals = { "ec2:ResourceTag/project" = "gusto-poc" } }
      },
      { Sid = "LookAtImagesAndTheFileSystem", Effect = "Allow", Action = ["ec2:DescribeImages", "elasticfilesystem:DescribeFileSystems"], Resource = "*" },
      {
        Sid    = "ReadTheUbuntuImageAndTheCellImage"
        Effect = "Allow"
        # A launch template that names its image by parameter is read with GetParameters when a host is launched from it.
        Action = ["ssm:GetParameter", "ssm:GetParameters"]
        Resource = [
          "arn:aws:ssm:${var.region}::parameter/aws/service/canonical/ubuntu/*",
          local.ssm_path,
        ]
      },
      { Sid = "PointTheCellTemplateAtTheImage", Effect = "Allow", Action = "ssm:PutParameter", Resource = "arn:aws:ssm:${var.region}:${local.account}:parameter/gusto/poc/cell-ami" },
    ]
  })
}

resource "aws_iam_role_policy" "ci" {
  name = "${local.ci}-launches-and-removes-demo-hosts"
  role = aws_iam_role.ci.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Sid = "Bundles", Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject"], Resource = ["${local.bucket}/releases/*", "${local.bucket}/image/*"] },
      { Sid = "BundlesList", Effect = "Allow", Action = ["s3:ListBucket"], Resource = local.bucket },
      {
        Sid      = "LaunchInstancesTaggedForTheProject"
        Effect   = "Allow"
        Action   = "ec2:RunInstances"
        Resource = ["arn:aws:ec2:${var.region}:${local.account}:instance/*", "arn:aws:ec2:${var.region}:${local.account}:volume/*"]
        Condition = {
          StringEquals = { "aws:RequestTag/project" = "gusto-poc" }
        }
      },
      {
        Sid    = "LaunchFromTheTemplateOnly"
        Effect = "Allow"
        Action = "ec2:RunInstances"
        Resource = [
          "arn:aws:ec2:${var.region}:${local.account}:launch-template/${aws_launch_template.host.id}",
          "arn:aws:ec2:${var.region}:${local.account}:launch-template/${aws_launch_template.role["orchestrator"].id}",
          "arn:aws:ec2:${var.region}:${local.account}:launch-template/${aws_launch_template.role["cell"].id}",
          "arn:aws:ec2:${var.region}:${local.account}:subnet/*",
          "arn:aws:ec2:${var.region}:${local.account}:network-interface/*",
          "arn:aws:ec2:${var.region}:${local.account}:security-group/${aws_security_group.host.id}",
          "arn:aws:ec2:${var.region}::image/*",
        ]
      },
      { Sid = "Tag", Effect = "Allow", Action = "ec2:CreateTags", Resource = "*", Condition = { StringEquals = { "ec2:CreateAction" = "RunInstances" } } },
      { Sid = "GiveTheHostItsRole", Effect = "Allow", Action = "iam:PassRole", Resource = aws_iam_role.host.arn, Condition = { StringEquals = { "iam:PassedToService" = "ec2.amazonaws.com" } } },
      { Sid = "Look", Effect = "Allow", Action = ["ec2:DescribeInstances", "ec2:DescribeInstanceStatus", "ec2:DescribeLaunchTemplates", "ec2:DescribeLaunchTemplateVersions", "ssm:DescribeInstanceInformation", "ssm:GetCommandInvocation"], Resource = "*" },
      {
        Sid      = "RemoveOnlyWhatIsTagged"
        Effect   = "Allow"
        Action   = "ec2:TerminateInstances"
        Resource = "arn:aws:ec2:${var.region}:${local.account}:instance/*"
        Condition = {
          StringEquals = { "ec2:ResourceTag/project" = "gusto-poc" }
        }
      },
      { Sid = "TellATaggedHostToPowerOff", Effect = "Allow", Action = "ssm:SendCommand", Resource = "arn:aws:ssm:${var.region}::document/AWS-RunShellScript" },
      {
        Sid       = "TellTheLiveFrontDoorToUpgrade"
        Effect    = "Allow"
        Action    = "ssm:SendCommand"
        Resource  = "arn:aws:ec2:${var.region}:${local.account}:instance/*"
        Condition = { StringEquals = { "ssm:resourceTag/project" = "gusto-live" } }
      },
      {
        Sid       = "TellATaggedHostToPowerOffOnlyTagged"
        Effect    = "Allow"
        Action    = "ssm:SendCommand"
        Resource  = "arn:aws:ec2:${var.region}:${local.account}:instance/*"
        Condition = { StringEquals = { "ssm:resourceTag/project" = "gusto-poc" } }
      },
    ]
  })
}

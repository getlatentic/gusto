# What outlives a demo host: the role and profile every host has, the security group, the
# launch template a host is made from, and the role GitHub Actions takes to launch and remove
# hosts. Nothing here costs while no host is up. A host is made from the launch template and
# removes itself, so no host is in this state.
#
# The CI role can launch and remove hosts tagged project=haas-poc and put objects in the
# bucket. It has no IAM permission, so it cannot change what it may do: this stack is applied by
# a person, with bootstrap.sh, and changes to it are rare.

data "aws_caller_identity" "here" {}

data "aws_partition" "here" {}

data "aws_vpc" "default" {
  default = true
}

locals {
  account  = data.aws_caller_identity.here.account_id
  host     = "haas-poc-demo-host"
  ci       = "haas-poc-ci"
  bucket   = "arn:aws:s3:::${var.bucket}"
  ssm_path = "arn:aws:ssm:${var.region}:${local.account}:parameter/haas/poc/*"
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

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
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
    tags          = { project = "haas-poc", name = "demo-host" }
  }

  tag_specifications {
    resource_type = "volume"
    tags          = { project = "haas-poc", name = "demo-host" }
  }
}

# ── GitHub Actions ───────────────────────────────────────────────────────────

# GitHub's identity provider is made once for the account, by somebody with the rights to, and
# is not managed here: bootstrap.sh says how. Its address is fixed.
locals {
  github_provider = "arn:${data.aws_partition.here.partition}:iam::${local.account}:oidc-provider/token.actions.githubusercontent.com"
}

resource "aws_iam_role" "ci" {
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
          "token.actions.githubusercontent.com:sub" = "repo:${var.repository}:ref:refs/heads/main"
        }
      }
    }]
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
        Resource = ["arn:aws:ec2:${var.region}:${local.account}:instance/*", "arn:aws:ec2:${var.region}:${local.account}:volume/*", "arn:aws:ec2:${var.region}:${local.account}:network-interface/*"]
        Condition = {
          StringEquals = { "aws:RequestTag/project" = "haas-poc" }
        }
      },
      {
        Sid    = "LaunchFromTheTemplateOnly"
        Effect = "Allow"
        Action = "ec2:RunInstances"
        Resource = [
          "arn:aws:ec2:${var.region}:${local.account}:launch-template/${aws_launch_template.host.id}",
          "arn:aws:ec2:${var.region}:${local.account}:subnet/*",
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
          StringEquals = { "ec2:ResourceTag/project" = "haas-poc" }
        }
      },
      { Sid = "TellATaggedHostToPowerOff", Effect = "Allow", Action = "ssm:SendCommand", Resource = "arn:aws:ssm:${var.region}::document/AWS-RunShellScript" },
      {
        Sid       = "TellATaggedHostToPowerOffOnlyTagged"
        Effect    = "Allow"
        Action    = "ssm:SendCommand"
        Resource  = "arn:aws:ec2:${var.region}:${local.account}:instance/*"
        Condition = { StringEquals = { "ssm:resourceTag/project" = "haas-poc" } }
      },
    ]
  })
}

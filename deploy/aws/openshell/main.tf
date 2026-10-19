# One instance for trying OpenShell's VM driver. It is not the E2B POC in the
# directory above: no DNS, no budget alarm, no E2B services, and its own state.
# Everything carries project=gusto-poc, the tag ../down.sh and this directory's
# down.sh look for.

data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}

data "aws_vpc" "default" {
  default = true
}

locals {
  tags = {
    project = "gusto-poc"
    name    = var.name
  }
}

resource "aws_key_pair" "spike" {
  key_name   = var.name
  public_key = var.public_key
  tags       = local.tags
}

resource "aws_security_group" "spike" {
  name        = var.name
  description = "OpenShell test: SSH from one address only"
  vpc_id      = data.aws_vpc.default.id
  tags        = local.tags

  ingress {
    description = "ssh"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.operator_cidrs
  }

  dynamic "ingress" {
    for_each = var.public_https ? [1] : []
    content {
      description = "https for the public cloud"
      from_port   = 443
      to_port     = 443
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

data "aws_caller_identity" "here" {}

# The instance reads the model's key from Parameter Store itself, so no key is
# ever an argument, a file on the machine that runs these scripts, or anything
# the scripts hold. It may read what is under /gusto/poc/ and nothing else.
resource "aws_iam_role" "spike" {
  name = var.name
  tags = local.tags
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "spike" {
  name = "${var.name}-parameters"
  role = aws_iam_role.spike.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Effect   = "Allow"
        Action   = ["ssm:GetParameter", "ssm:GetParameters"]
        Resource = "arn:aws:ssm:${var.region}:${data.aws_caller_identity.here.account_id}:parameter/gusto/poc/*"
      }],
      var.demo_bucket == "" ? [] : [
        { Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject"], Resource = "arn:aws:s3:::${var.demo_bucket}/*" },
        { Effect = "Allow", Action = ["s3:ListBucket"], Resource = "arn:aws:s3:::${var.demo_bucket}" },
      ]
    )
  })
}

resource "aws_iam_instance_profile" "spike" {
  name = var.name
  role = aws_iam_role.spike.name
  tags = local.tags
}

resource "aws_instance" "spike" {
  ami                  = data.aws_ssm_parameter.ubuntu.value
  instance_type        = var.instance_type
  key_name             = aws_key_pair.spike.key_name
  iam_instance_profile = aws_iam_instance_profile.spike.name
  # A host that powers itself off is removed by that, and its disk with it.
  instance_initiated_shutdown_behavior = var.self_terminate ? "terminate" : "stop"
  vpc_security_group_ids               = [aws_security_group.spike.id]
  tags                                 = local.tags
  volume_tags                          = local.tags

  cpu_options {
    nested_virtualization = "enabled"
  }

  metadata_options {
    http_tokens = "required"
  }

  root_block_device {
    volume_size           = 40
    volume_type           = "gp3"
    delete_on_termination = true
  }
}

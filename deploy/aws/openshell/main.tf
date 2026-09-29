# One instance for trying OpenShell's VM driver. It is not the E2B POC in the
# directory above: no DNS, no budget alarm, no E2B services, and its own state.
# Everything carries project=haas-poc, the tag ../down.sh and this directory's
# down.sh look for.

data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}

data "aws_vpc" "default" {
  default = true
}

locals {
  tags = {
    project = "haas-poc"
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

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "spike" {
  ami                    = data.aws_ssm_parameter.ubuntu.value
  instance_type          = var.instance_type
  key_name               = aws_key_pair.spike.key_name
  vpc_security_group_ids = [aws_security_group.spike.id]
  tags                   = local.tags
  volume_tags            = local.tags

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

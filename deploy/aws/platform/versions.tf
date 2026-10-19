terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      # 6.33 is the first release with cpu_options.nested_virtualization.
      version = ">= 6.33"
    }
  }
  # The bucket is the one demo-store.sh makes, which no stack here removes. bootstrap.sh
  # gives the bucket's name at init.
  backend "s3" {
    key          = "terraform/platform.tfstate"
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = { project = "gusto-poc", stack = "platform" }
  }
}

terraform {
  required_version = ">= 1.7.5"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      # 6.33 is the first release with cpu_options.nested_virtualization.
      version = ">= 6.33"
    }
  }
}

provider "aws" {
  region = var.region
}

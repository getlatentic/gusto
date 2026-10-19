terraform {
  required_version = ">= 1.7.5"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      # 6.33 is the first release with cpu_options.nested_virtualization,
      # which the E2B module sets on the instance.
      version = ">= 6.33"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# The token is read from CLOUDFLARE_API_TOKEN, which up.sh sets from
# ~/.config/gusto/cloudflare_token, so it is never in a file here.
provider "cloudflare" {}

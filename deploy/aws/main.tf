locals {
  name = "haas-poc"
  tags = { project = "haas-poc" }
  # The module reports its address only inside its URLs.
  address = regex("^http://([0-9.]+):", module.e2b.api_url)[0]
}

# E2B Embed on one instance with nested virtualization, from E2B's own module,
# pinned to the commit this was written against. Its resources are named
# haas-poc, which the IAM user's inline policy is scoped to.
module "e2b" {
  source = "github.com/e2b-dev/runtime//embed/terraform/aws?ref=cb88df44c7b6ef7df1ab63d51266e645d19880d8"

  name                = local.name
  instance_type       = var.instance_type
  client_cidrs        = var.operator_cidrs
  hugepages           = var.hugepages
  root_volume_size_gb = 100
  tags                = local.tags
}

data "cloudflare_zone" "this" {
  filter = { name = var.zone }
}

# DNS only, not proxied: the name has to reach the instance itself for its
# certificate to be issued there.
resource "cloudflare_dns_record" "poc" {
  zone_id = data.cloudflare_zone.this.zone_id
  name    = "${var.hostname}.${var.zone}"
  type    = "A"
  content = local.address
  ttl     = 60
  proxied = false
  comment = "haas POC; made and removed by deploy/aws"
}

resource "aws_budgets_budget" "poc" {
  name         = local.name
  budget_type  = "COST"
  limit_amount = tostring(var.budget_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_email]
  }
}

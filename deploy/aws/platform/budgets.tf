# Two budgets that tell the owner of spending that has run away, in the account the public cloud is in. They count usage before
# credits, since credits hide a bill until they run out, and send mail. The account has an older budget of five dollars that is
# always over, which is why it is not the one to watch.

locals {
  alerts = var.alert_email == "" ? [] : [var.alert_email]
}

resource "aws_budgets_budget" "monthly" {
  count        = length(local.alerts) > 0 ? 1 : 0
  name         = "gusto-monthly-usage"
  budget_type  = "COST"
  limit_amount = tostring(var.monthly_budget_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  cost_types {
    include_credit = false
    include_refund = false
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = local.alerts
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = local.alerts
  }
}

resource "aws_budgets_budget" "daily" {
  count        = length(local.alerts) > 0 ? 1 : 0
  name         = "gusto-daily-usage"
  budget_type  = "COST"
  limit_amount = tostring(var.daily_budget_usd)
  limit_unit   = "USD"
  time_unit    = "DAILY"

  cost_types {
    include_credit = false
    include_refund = false
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = local.alerts
  }
}

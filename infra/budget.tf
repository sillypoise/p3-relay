variable "budget_alert_email" {
  description = "Private ASCII alert mailbox; empty disables the budget, never enables runtime."
  type        = string
  default     = ""
  sensitive   = true
  validation {
    condition = var.budget_alert_email == "" || (
      length(var.budget_alert_email) <= 254 &&
      !strcontains(var.budget_alert_email, "..") &&
      can(regex(join("", [
        "^[A-Za-z0-9_%+-]([A-Za-z0-9._%+-]{0,62}[A-Za-z0-9_%+-])?@",
        "([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\\.)+",
        "[A-Za-z]{2,63}$"
      ]), var.budget_alert_email))
    )
    error_message = "Provide an ASCII mailbox with valid DNS labels, or leave it empty."
  }
}

# Account-wide costs avoid silently missing unactivated tags and Express-managed resources.
# This adds a notification-only budget; it changes no existing budgets or other project resources.
resource "aws_budgets_budget" "relay" {
  count        = nonsensitive(var.budget_alert_email != "") ? 1 : 0
  account_id   = var.aws_account_id
  name         = "p3-relay-aws-monthly-guardrail"
  budget_type  = "COST"
  limit_amount = "50"
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  cost_types {
    include_credit             = false
    include_refund             = false
    include_discount           = true
    include_other_subscription = true
    include_recurring          = true
    include_subscription       = true
    include_support            = true
    include_tax                = true
    include_upfront            = true
    use_amortized              = false
    use_blended                = false
  }

  dynamic "notification" {
    for_each = {
      early    = { type = "ACTUAL", percentage = 70 }
      warning  = { type = "ACTUAL", percentage = 90 }
      ceiling  = { type = "ACTUAL", percentage = 100 }
      forecast = { type = "FORECASTED", percentage = 100 }
    }
    content {
      comparison_operator        = "GREATER_THAN"
      threshold                  = notification.value.percentage
      threshold_type             = "PERCENTAGE"
      notification_type          = notification.value.type
      subscriber_email_addresses = [var.budget_alert_email]
    }
  }
}

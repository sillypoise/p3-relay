# Mock checks cover the disabled default, cost coverage, alert boundaries, and invalid mailboxes.
# They cannot prove AWS email delivery or forecast availability in a new account.
mock_provider "aws" {
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/mock-relay" }
  }
}
variables { aws_account_id = "123456789012" }

run "disabled_without_mailbox" {
  command = plan
  assert {
    condition     = length(aws_budgets_budget.relay) == 0
    error_message = "An empty mailbox must not create a budget without subscribers."
  }
}

run "bounded_account_guardrail" {
  command = apply
  variables { budget_alert_email = "alerts@example.invalid" }
  assert {
    condition = (
      aws_budgets_budget.relay[0].limit_amount == "50" &&
      aws_budgets_budget.relay[0].limit_unit == "USD" &&
      aws_budgets_budget.relay[0].time_unit == "MONTHLY" &&
      length(aws_budgets_budget.relay[0].cost_filter) == 0 &&
      !aws_budgets_budget.relay[0].cost_types[0].include_credit &&
      !aws_budgets_budget.relay[0].cost_types[0].include_refund &&
      aws_budgets_budget.relay[0].cost_types[0].include_tax
    )
    error_message = "Budget must cover all account costs without credit/refund masking."
  }
  assert {
    condition = (
      length(aws_budgets_budget.relay[0].notification) == 4 &&
      toset([for notice in aws_budgets_budget.relay[0].notification :
        "${notice.notification_type}:${notice.threshold}"]
      ) == toset(["ACTUAL:70", "ACTUAL:90", "ACTUAL:100", "FORECASTED:100"]) &&
      alltrue([for notice in aws_budgets_budget.relay[0].notification :
        notice.subscriber_email_addresses == toset([var.budget_alert_email]) &&
        notice.comparison_operator == "GREATER_THAN" && notice.threshold_type == "PERCENTAGE"
      ])
    )
    error_message = "Preserve the four explicit thresholds and the configured subscriber."
  }
}

run "reject_service_without_budget" {
  command = plan
  variables {
    deploy_service       = true
    runtime_image_digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }
  expect_failures = [aws_cloudformation_stack.runtime]
}

run "reject_invalid_mailbox" {
  command = plan
  variables { budget_alert_email = "alerts@-invalid.example" }
  expect_failures = [var.budget_alert_email]
}

run "reject_header_injection" {
  command = plan
  variables { budget_alert_email = "alerts@example.invalid\nother@example.invalid" }
  expect_failures = [var.budget_alert_email]
}

run "reject_leading_dot" {
  command = plan
  variables { budget_alert_email = ".alerts@example.invalid" }
  expect_failures = [var.budget_alert_email]
}

run "reject_consecutive_dots" {
  command = plan
  variables { budget_alert_email = "alerts..other@example.invalid" }
  expect_failures = [var.budget_alert_email]
}

run "accept_maximum_local_part" {
  command = plan
  variables { budget_alert_email = "${join("", [for index in range(64) : "a"])}@example.invalid" }
}

run "reject_excessive_local_part" {
  command = plan
  variables { budget_alert_email = "${join("", [for index in range(65) : "a"])}@example.invalid" }
  expect_failures = [var.budget_alert_email]
}

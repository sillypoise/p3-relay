# Offline checks cover disabled/malformed configuration, fixed reminder boundaries and scoped delivery.
# They cannot prove SNS confirmation, inbox receipt or future scheduled execution.
mock_provider "aws" {
  mock_resource "aws_sns_topic" {
    defaults = { arn = "arn:aws:sns:us-east-1:123456789012:p3-relay-operator-alerts" }
  }
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/p3-relay-expiry-scheduler" }
  }
  mock_resource "aws_scheduler_schedule_group" {
    defaults = { arn = "arn:aws:scheduler:us-east-1:123456789012:schedule-group/p3-relay-expiry" }
  }
  mock_resource "aws_cloudwatch_metric_alarm" {
    defaults = {
      arn = "arn:aws:cloudwatch:us-east-1:123456789012:alarm:p3-relay-expiry-delivery-failed"
    }
  }
}
variables { aws_account_id = "123456789012" }

run "disabled_without_identity" {
  command = plan
  assert {
    condition = (
      length(aws_sns_topic.operator_alerts) == 0 &&
      length(aws_scheduler_schedule.expiry) == 0 &&
      length(aws_cloudwatch_metric_alarm.expiry_delivery) == 0
    )
    error_message = "An unset identity must not create expiry resources."
  }
}

run "bounded_expiry_delivery" {
  command = apply
  variables {
    budget_alert_email             = "alerts@example.invalid"
    gateway_certificate_expires_at = "2030-12-13T01:24:40Z"
  }
  assert {
    condition = toset([for schedule in aws_scheduler_schedule.expiry :
      schedule.schedule_expression]) == toset([
      "at(2030-11-13T01:24:40)", "at(2030-12-06T01:24:40)", "at(2030-12-12T01:24:40)"
    ])
    error_message = "Reminder times must be exactly 30, seven and one day before expiry."
  }
  assert {
    condition = alltrue([for schedule in aws_scheduler_schedule.expiry :
      schedule.schedule_expression_timezone == "UTC" &&
      schedule.flexible_time_window[0].mode == "OFF" &&
      schedule.action_after_completion == "NONE" &&
      schedule.target[0].arn == "arn:aws:scheduler:::aws-sdk:sns:publish" &&
      schedule.target[0].retry_policy[0].maximum_retry_attempts == 3 &&
      schedule.target[0].retry_policy[0].maximum_event_age_in_seconds == 3600 &&
      jsondecode(schedule.target[0].input).TopicArn == aws_sns_topic.operator_alerts[0].arn
    ])
    error_message = "Targets, retry exhaustion and retained schedule ownership must stay bounded."
  }
  assert {
    condition = (
      jsondecode(aws_iam_role_policy.expiry_publish[0].policy).Statement[0].Resource ==
      aws_sns_topic.operator_alerts[0].arn &&
      jsondecode(aws_iam_role.expiry_scheduler[0].assume_role_policy).Statement[0].Condition.
      ArnEquals["aws:SourceArn"] == aws_scheduler_schedule_group.expiry[0].arn &&
      aws_sns_topic_subscription.operator_alerts[0].protocol == "email" &&
      aws_sns_topic_subscription.operator_alerts[0].endpoint == var.budget_alert_email
    )
    error_message = "Delivery must stay scoped to this group/topic and the private mailbox."
  }
  assert {
    condition = (
      aws_cloudwatch_metric_alarm.expiry_delivery[0].metric_name == "InvocationDroppedCount" &&
      aws_cloudwatch_metric_alarm.expiry_delivery[0].threshold == 0 &&
      aws_cloudwatch_metric_alarm.expiry_delivery[0].treat_missing_data == "notBreaching" &&
      jsondecode(aws_sns_topic_policy.operator_alerts[0].policy).Statement[2].Effect == "Deny"
    )
    error_message = "Exhausted delivery needs an independent alarm and TLS-only topic access."
  }
}

run "reject_absent_mailbox" {
  command = plan
  variables { gateway_certificate_expires_at = "2030-12-13T01:24:40Z" }
  expect_failures = [aws_sns_topic.operator_alerts]
}

run "reject_invalid_calendar_date" {
  command = plan
  variables { gateway_certificate_expires_at = "2030-02-30T01:24:40Z" }
  expect_failures = [var.gateway_certificate_expires_at]
}

run "reject_non_utc_input" {
  command = plan
  variables { gateway_certificate_expires_at = "2030-12-13T01:24:40+02:00" }
  expect_failures = [var.gateway_certificate_expires_at]
}

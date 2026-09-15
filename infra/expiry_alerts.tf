variable "gateway_certificate_expires_at" {
  description = "Reviewed gateway CA/password expiry in UTC; empty disables expiry reminders."
  type        = string
  default     = ""
  validation {
    condition = var.gateway_certificate_expires_at == "" || (
      can(regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$",
      var.gateway_certificate_expires_at)) &&
      can(timeadd(var.gateway_certificate_expires_at, "0h"))
    )
    error_message = "Use a valid UTC RFC3339 timestamp ending in Z, or leave expiry disabled."
  }
}

locals {
  # SNS topic policies reject service-wide wildcards; enumerate supported topic actions.
  operator_topic_actions = [
    "SNS:Publish", "SNS:Subscribe", "SNS:GetTopicAttributes", "SNS:SetTopicAttributes",
    "SNS:DeleteTopic", "SNS:AddPermission", "SNS:RemovePermission", "SNS:ListSubscriptionsByTopic"
  ]
  expiry_alerts_enabled = var.gateway_certificate_expires_at != ""
  expiry_reminders = local.expiry_alerts_enabled ? {
    thirty_days = "-720h"
    seven_days  = "-168h"
    one_day     = "-24h"
  } : {}
}

resource "aws_sns_topic" "operator_alerts" {
  count = local.expiry_alerts_enabled ? 1 : 0
  name  = "p3-relay-operator-alerts"
  lifecycle {
    precondition {
      condition     = var.budget_alert_email != ""
      error_message = "Expiry alerts require the existing private operator mailbox."
    }
  }
}

resource "aws_sns_topic_subscription" "operator_alerts" {
  count     = local.expiry_alerts_enabled ? 1 : 0
  topic_arn = aws_sns_topic.operator_alerts[0].arn
  protocol  = "email"
  endpoint  = var.budget_alert_email
}

resource "aws_scheduler_schedule_group" "expiry" {
  count = local.expiry_alerts_enabled ? 1 : 0
  name  = "p3-relay-expiry"
}

resource "aws_iam_role" "expiry_scheduler" {
  count = local.expiry_alerts_enabled ? 1 : 0
  name  = "p3-relay-expiry-scheduler"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow", Action = "sts:AssumeRole"
      Principal = { Service = "scheduler.amazonaws.com" }
      Condition = {
        StringEquals = { "aws:SourceAccount" = var.aws_account_id }
        ArnEquals    = { "aws:SourceArn" = aws_scheduler_schedule_group.expiry[0].arn }
      }
    }]
  })
}

resource "aws_iam_role_policy" "expiry_publish" {
  count = local.expiry_alerts_enabled ? 1 : 0
  name  = "publish-relay-expiry-only"
  role  = aws_iam_role.expiry_scheduler[0].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow", Action = "sns:Publish", Resource = aws_sns_topic.operator_alerts[0].arn
    }]
  })
}

# Direct SNS targets avoid a Lambda/runtime merely to send three public-metadata reminders.
# Keep completed schedules in state; rotation updates their dates rather than recreating deleted jobs.
resource "aws_scheduler_schedule" "expiry" {
  for_each                     = local.expiry_reminders
  name                         = "p3-relay-expiry-${replace(each.key, "_", "-")}"
  group_name                   = aws_scheduler_schedule_group.expiry[0].name
  schedule_expression_timezone = "UTC"
  schedule_expression = "at(${formatdate("YYYY-MM-DD'T'hh:mm:ss",
  timeadd(var.gateway_certificate_expires_at, each.value))})"
  state                   = "ENABLED"
  action_after_completion = "NONE"
  flexible_time_window { mode = "OFF" }
  target {
    arn      = "arn:aws:scheduler:::aws-sdk:sns:publish"
    role_arn = aws_iam_role.expiry_scheduler[0].arn
    input = jsonencode({
      TopicArn = aws_sns_topic.operator_alerts[0].arn
      Subject  = "Relay certificate and database credential expiry"
      Message = join(" ", [
        "Relay gateway CA and database passwords expire at ${var.gateway_certificate_expires_at}.",
        "Renew and verify both TLS trust and credentials before expiry.",
        "Existing sessions require explicit draining; see ops/README.md and the TLS contract."
      ])
    })
    retry_policy {
      maximum_event_age_in_seconds = 3600
      maximum_retry_attempts       = 3
    }
  }
  depends_on = [aws_iam_role_policy.expiry_publish]
}

# Failed scheduler delivery has an independent publisher identity, rather than silently disappearing.
resource "aws_cloudwatch_metric_alarm" "expiry_delivery" {
  count               = local.expiry_alerts_enabled ? 1 : 0
  alarm_name          = "p3-relay-expiry-delivery-failed"
  alarm_description   = "A Relay expiry reminder exhausted scheduler delivery. Investigate immediately."
  namespace           = "AWS/Scheduler"
  metric_name         = "InvocationDroppedCount"
  dimensions          = { ScheduleGroup = aws_scheduler_schedule_group.expiry[0].name }
  statistic           = "Sum"
  period              = 60
  evaluation_periods  = 1
  datapoints_to_alarm = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.operator_alerts[0].arn]
}

resource "aws_sns_topic_policy" "operator_alerts" {
  count = local.expiry_alerts_enabled ? 1 : 0
  arn   = aws_sns_topic.operator_alerts[0].arn
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "Owner", Effect = "Allow", Action = local.operator_topic_actions
        Principal = { AWS = "arn:aws:iam::${var.aws_account_id}:root" }
        Resource  = aws_sns_topic.operator_alerts[0].arn
      },
      {
        Sid       = "DeliveryAlarm", Effect = "Allow", Action = "sns:Publish"
        Principal = { Service = "cloudwatch.amazonaws.com" }
        Resource  = aws_sns_topic.operator_alerts[0].arn
        Condition = {
          StringEquals = { "aws:SourceAccount" = var.aws_account_id }
          ArnEquals    = { "aws:SourceArn" = aws_cloudwatch_metric_alarm.expiry_delivery[0].arn }
        }
      },
      {
        Sid       = "DenyInsecureTransport", Effect = "Deny", Action = local.operator_topic_actions
        Principal = "*", Resource = aws_sns_topic.operator_alerts[0].arn
        Condition = { Bool = { "aws:SecureTransport" = "false" } }
      }
    ]
  })
}

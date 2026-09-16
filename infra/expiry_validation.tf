variable "gateway_alert_validation_at" {
  description = "Optional one-time UTC alert validation date; retain completed evidence until teardown."
  type        = string
  default     = ""
  validation {
    condition = var.gateway_alert_validation_at == "" || (
      var.gateway_certificate_expires_at != "" &&
      can(regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$",
      var.gateway_alert_validation_at)) &&
      can(timeadd(var.gateway_alert_validation_at, "0h"))
    )
    error_message = "Validation requires an enabled expiry channel and a valid UTC timestamp."
  }
}

# Never move a production reminder to test it. This one-time job exercises the same publisher.
# Retain the completed job (no recurrence or idle compute); remove at approved infrastructure teardown.
resource "aws_scheduler_schedule" "expiry_validation" {
  count                        = var.gateway_alert_validation_at != "" ? 1 : 0
  name                         = "p3-relay-expiry-validation"
  group_name                   = aws_scheduler_schedule_group.expiry[0].name
  schedule_expression_timezone = "UTC"
  schedule_expression = "at(${formatdate("YYYY-MM-DD'T'hh:mm:ss",
  var.gateway_alert_validation_at)})"
  state                   = "ENABLED"
  action_after_completion = "NONE"
  flexible_time_window { mode = "OFF" }
  target {
    arn      = "arn:aws:scheduler:::aws-sdk:sns:publish"
    role_arn = aws_iam_role.expiry_scheduler[0].arn
    input = jsonencode({
      TopicArn = aws_sns_topic.operator_alerts[0].arn
      Subject  = "TEST: Relay scheduled expiry alert delivery"
      Message = join(" ", [
        "TEST ONLY: one-time Relay Scheduler-to-SNS delivery validation, not an expiry incident.",
        "Please confirm receipt to the deployment operator without sharing private information."
      ])
    })
    retry_policy {
      maximum_event_age_in_seconds = 3600
      maximum_retry_attempts       = 3
    }
  }
  depends_on = [aws_iam_role_policy.expiry_publish, aws_sns_topic_policy.operator_alerts]
}

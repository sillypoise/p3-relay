# Mocked providers exercise contract values without AWS calls or real resources.
mock_provider "aws" {}

variables { aws_account_id = "123456789012" }

run "bounded_encrypted_notifications" {
  command = apply

  assert {
    condition = (
      aws_sqs_queue.notifications.fifo_queue == false &&
      aws_sqs_queue.dead_letters.fifo_queue == false &&
      aws_sqs_queue.notifications.visibility_timeout_seconds == 300 &&
      aws_sqs_queue.notifications.receive_wait_time_seconds == 10 &&
      aws_sqs_queue.notifications.max_message_size == 1024 &&
      aws_sqs_queue.notifications.message_retention_seconds == 86400
    )
    error_message = "Source queue settings must match the runtime notification contract."
  }
  assert {
    condition = (
      aws_sqs_queue.notifications.sqs_managed_sse_enabled &&
      aws_sqs_queue.dead_letters.sqs_managed_sse_enabled &&
      aws_sqs_queue.dead_letters.message_retention_seconds == 1209600
    )
    error_message = "Both queues require encryption; dead letters require fourteen-day retention."
  }
  assert {
    condition = (
      jsondecode(aws_sqs_queue.notifications.redrive_policy).maxReceiveCount == 5 &&
      jsondecode(aws_sqs_queue.notifications.redrive_policy).deadLetterTargetArn ==
      aws_sqs_queue.dead_letters.arn &&
      jsondecode(
        aws_sqs_queue_redrive_allow_policy.source_only.redrive_allow_policy
      ).redrivePermission == "byQueue" &&
      jsondecode(
        aws_sqs_queue_redrive_allow_policy.source_only.redrive_allow_policy
      ).sourceQueueArns == [aws_sqs_queue.notifications.arn]
    )
    error_message = "Redrive must use the designated DLQ after five receives, from only the source."
  }
  assert {
    condition = alltrue([
      for policy in aws_sqs_queue_policy.tls :
      jsondecode(policy.policy).Statement[0].Effect == "Deny" &&
      jsondecode(policy.policy).Statement[0].Condition.Bool["aws:SecureTransport"] == "false"
    ])
    error_message = "Neither queue may accept insecure transport."
  }
  assert {
    condition = (
      aws_ecr_repository.runtime.image_tag_mutability == "IMMUTABLE" &&
      aws_ecr_repository.runtime.force_delete == false &&
      aws_secretsmanager_secret.runtime.recovery_window_in_days == 30 &&
      aws_cloudwatch_log_group.runtime.retention_in_days == 7
    )
    error_message = "Release immutability, recovery, and bounded log retention must remain enabled."
  }
}

run "reject_malformed_account" {
  command = plan
  variables { aws_account_id = "not-an-account" }
  expect_failures = [var.aws_account_id]
}

run "reject_account_length_boundary" {
  command = plan
  variables { aws_account_id = "1234567890123" }
  expect_failures = [var.aws_account_id]
}

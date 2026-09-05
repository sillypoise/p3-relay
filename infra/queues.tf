resource "aws_sqs_queue" "dead_letters" {
  name                      = "p3-relay-notification-dlq"
  fifo_queue                = false
  message_retention_seconds = 1209600
  max_message_size          = 1024
  sqs_managed_sse_enabled   = true
}

resource "aws_sqs_queue" "notifications" {
  name                       = "p3-relay-notifications"
  fifo_queue                 = false
  visibility_timeout_seconds = 300
  receive_wait_time_seconds  = 10
  message_retention_seconds  = 86400
  max_message_size           = 1024
  delay_seconds              = 0
  sqs_managed_sse_enabled    = true
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dead_letters.arn
    maxReceiveCount     = 5
  })
}

resource "aws_sqs_queue_redrive_allow_policy" "source_only" {
  queue_url = aws_sqs_queue.dead_letters.id
  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue"
    sourceQueueArns   = [aws_sqs_queue.notifications.arn]
  })
}

# Deny insecure transport even if a future identity policy accidentally grants broader access.
resource "aws_sqs_queue_policy" "tls" {
  for_each = {
    notifications = { url = aws_sqs_queue.notifications.id, arn = aws_sqs_queue.notifications.arn }
    dead_letters  = { url = aws_sqs_queue.dead_letters.id, arn = aws_sqs_queue.dead_letters.arn }
  }
  queue_url = each.value.url
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport", Effect = "Deny", Principal = "*", Action = "sqs:*"
      Resource  = each.value.arn
      Condition = { Bool = { "aws:SecureTransport" = "false" } }
    }]
  })
}

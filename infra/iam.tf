locals {
  task_trust = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow", Action = "sts:AssumeRole"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Condition = {
        StringEquals = { "aws:SourceAccount" = var.aws_account_id }
        ArnLike      = { "aws:SourceArn" = "arn:aws:ecs:us-east-1:${var.aws_account_id}:*" }
      }
    }]
  })
  execution_secrets = {
    runtime   = aws_secretsmanager_secret.runtime.arn
    migration = aws_secretsmanager_secret.migration.arn
  }
}

# ECS's agent reads secrets; application processes receive only their selected JSON fields.
resource "aws_iam_role" "execution" {
  for_each             = local.execution_secrets
  name                 = "p3-relay-${each.key}-execution"
  assume_role_policy   = local.task_trust
  max_session_duration = 3600
}

resource "aws_iam_role_policy" "execution" {
  for_each = local.execution_secrets
  name     = "p3-relay-${each.key}-execution"
  role     = aws_iam_role.execution[each.key].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      # ECR does not support resource scoping for authorization-token requests.
      { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
      {
        Effect = "Allow"
        Action = [
          "ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage"
        ]
        Resource = aws_ecr_repository.runtime.arn
      },
      {
        Effect   = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.runtime.arn}:log-stream:*"
      },
      { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = each.value }
    ]
  })
}

# A task has one IAM principal: the co-located API and worker share these hint-only capabilities.
resource "aws_iam_role" "runtime" {
  name                 = "p3-relay-runtime"
  assume_role_policy   = local.task_trust
  max_session_duration = 3600
}

resource "aws_iam_role_policy" "runtime" {
  name = "p3-relay-notifications"
  role = aws_iam_role.runtime.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "sqs:GetQueueAttributes", "sqs:SendMessage", "sqs:ReceiveMessage", "sqs:DeleteMessage"
      ]
      Resource = aws_sqs_queue.notifications.arn
    }]
  })
}

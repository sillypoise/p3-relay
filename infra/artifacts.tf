resource "aws_ecr_repository" "runtime" {
  name                 = "p3-relay"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false
  encryption_configuration { encryption_type = "AES256" }
  image_scanning_configuration { scan_on_push = true }
}

# Keep tagged releases for deliberate rollback; expire only untagged build artifacts.
resource "aws_ecr_lifecycle_policy" "untagged" {
  repository = aws_ecr_repository.runtime.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1, description = "Expire untagged artifacts after seven days"
      selection = {
        tagStatus = "untagged", countType = "sinceImagePushed", countUnit = "days", countNumber = 7
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_cloudwatch_log_group" "runtime" {
  name              = "/p3-relay/runtime"
  retention_in_days = 7
}

# Only metadata belongs in state. Secret versions are populated through a controlled channel.
resource "aws_secretsmanager_secret" "runtime" {
  name                    = "p3-relay/runtime"
  recovery_window_in_days = 30
}

output "repository_url" { value = aws_ecr_repository.runtime.repository_url }
output "notification_queue_url" { value = aws_sqs_queue.notifications.url }
output "runtime_secret_arn" { value = aws_secretsmanager_secret.runtime.arn }

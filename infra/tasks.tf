variable "runtime_image_digest" {
  description = "Reviewed image digest in Relay's ECR repository; empty leaves tasks unregistered."
  type        = string
  default     = ""
  validation {
    condition = (
      var.runtime_image_digest == "" ||
      can(regex("^sha256:[0-9a-f]{64}$", var.runtime_image_digest))
    )
    error_message = "Use an immutable sha256 digest, not a tag or external repository URL."
  }
}

variable "sandbox_origin" {
  description = "Verified lowercase HTTPS origin; empty disables visitor routes during bootstrap."
  type        = string
  default     = ""
  validation {
    condition = (
      var.sandbox_origin == "" ||
      (length(var.sandbox_origin) <= 261 && can(regex(join("", [
        "^https://[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?",
        "(\\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$"
      ]), var.sandbox_origin)))
    )
    error_message = "Use a lowercase HTTPS hostname only, without credentials, port, path or query."
  }
}

locals {
  runtime_image = "${aws_ecr_repository.runtime.repository_url}@${var.runtime_image_digest}"
  runtime_environment = [
    { name = "RELAY_SQS_QUEUE_URL", value = aws_sqs_queue.notifications.url },
    { name = "RELAY_SQS_REGION", value = "us-east-1" },
    { name = "RELAY_ALLOW_PRIVATE_DESTINATIONS", value = "false" },
    { name = "AWS_EC2_METADATA_DISABLED", value = "true" },
    { name = "GOMEMLIMIT", value = "160MiB" }
  ]
  api_secret_fields = {
    RELAY_DATABASE_URL    = "database_url"
    RELAY_SOURCE_KEY      = "source_key"
    RELAY_INGRESS_SECRET  = "ingress_secret"
    RELAY_OPERATOR_TOKEN  = "operator_token"
    RELAY_DESTINATION_URL = "destination_url"
    RELAY_SANDBOX_KEY     = "sandbox_key"
  }
  worker_secret_fields = {
    RELAY_DATABASE_URL    = "database_url"
    RELAY_DELIVERY_SECRET = "delivery_secret"
  }
  container_protection = {
    essential              = true
    user                   = "10001:10001"
    readonlyRootFilesystem = true
    privileged             = false
    linuxParameters        = { capabilities = { drop = ["ALL"] } }
    stopTimeout            = 30
  }
}

# Registration alone starts no compute. Express Mode service creation remains a separate gate.
resource "aws_ecs_task_definition" "runtime" {
  count                    = var.runtime_image_digest == "" ? 0 : 1
  family                   = "p3-relay-runtime"
  skip_destroy             = false
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution["runtime"].arn
  task_role_arn            = aws_iam_role.runtime.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }
  container_definitions = jsonencode([
    merge(local.container_protection, {
      name    = "Main", image = local.runtime_image, cpu = 128, memory = 256
      command = ["/usr/local/bin/relay-api"]
      portMappings = [{
        containerPort = 8080, protocol = "tcp", name = "http", appProtocol = "http"
      }]
      environment = concat(local.runtime_environment, [
        { name = "RELAY_HTTP_ADDRESS", value = "0.0.0.0:8080" },
        { name = "RELAY_WEB_DIRECTORY", value = "/app/web" },
        { name = "RELAY_SANDBOX_ORIGIN", value = var.sandbox_origin }
      ])
      secrets = [for name, field in local.api_secret_fields : {
        name = name, valueFrom = "${aws_secretsmanager_secret.runtime.arn}:${field}::"
      }]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.runtime.name
          awslogs-region        = "us-east-1"
          awslogs-stream-prefix = "api"
          mode                  = "blocking"
        }
      }
    }),
    merge(local.container_protection, {
      name        = "Worker", image = local.runtime_image, cpu = 128, memory = 256
      command     = ["/usr/local/bin/relay-worker"]
      environment = local.runtime_environment
      secrets = [for name, field in local.worker_secret_fields : {
        name = name, valueFrom = "${aws_secretsmanager_secret.runtime.arn}:${field}::"
      }]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.runtime.name
          awslogs-region        = "us-east-1"
          awslogs-stream-prefix = "worker"
          mode                  = "blocking"
        }
      }
    })
  ])
  depends_on = [aws_iam_role_policy.execution, aws_iam_role_policy.runtime]
}

resource "aws_ecs_task_definition" "migration" {
  count                    = var.runtime_image_digest == "" ? 0 : 1
  family                   = "p3-relay-migration"
  skip_destroy             = false
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution["migration"].arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }
  container_definitions = jsonencode([merge(local.container_protection, {
    name             = "Migration", image = local.runtime_image, cpu = 256, memory = 512
    command          = ["/usr/local/bin/relay-migrate"]
    workingDirectory = "/app"
    secrets = [{
      name      = "RELAY_DATABASE_URL"
      valueFrom = "${aws_secretsmanager_secret.migration.arn}:database_url::"
    }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.runtime.name
        awslogs-region        = "us-east-1"
        awslogs-stream-prefix = "migration"
        mode                  = "blocking"
      }
    }
  })])
  depends_on = [aws_iam_role_policy.execution]
}

output "runtime_task_definition_arn" {
  value = try(aws_ecs_task_definition.runtime[0].arn, null)
}
output "migration_task_definition_arn" {
  value = try(aws_ecs_task_definition.migration[0].arn, null)
}

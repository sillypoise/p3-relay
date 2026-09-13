# These are configuration checks, not live IAM enforcement or memory measurements.
mock_provider "aws" {
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/mock-relay" }
  }
}
variables { aws_account_id = "123456789012" }

run "no_image_no_tasks" {
  command = plan
  assert {
    condition = (
      length(aws_ecs_task_definition.runtime) == 0 &&
      length(aws_ecs_task_definition.migration) == 0
    )
    error_message = "An empty image digest must leave both task definitions unregistered."
  }
}

run "bounded_tasks_and_scoped_credentials" {
  command = apply
  variables {
    runtime_image_digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    sandbox_origin       = "https://relay.example.com"
  }
  assert {
    condition = (
      aws_ecs_task_definition.runtime[0].cpu == "256" &&
      aws_ecs_task_definition.runtime[0].memory == "512" &&
      aws_ecs_task_definition.runtime[0].network_mode == "awsvpc" &&
      length(jsondecode(aws_ecs_task_definition.runtime[0].container_definitions)) == 2
    )
    error_message = "The shared task must preserve the reviewed compute and container bounds."
  }
  assert {
    condition = alltrue([
      for container in jsondecode(aws_ecs_task_definition.runtime[0].container_definitions) :
      container.essential && container.readonlyRootFilesystem &&
      container.user == "10001:10001" && container.privileged == false &&
      container.linuxParameters.capabilities.drop == ["ALL"] && container.stopTimeout == 30 &&
      container.memory == 256 && container.cpu == 128 &&
      container.logConfiguration.options.mode == "blocking" &&
      endswith(container.image, "@${var.runtime_image_digest}") &&
      contains(container.environment, {
        name = "RELAY_ALLOW_PRIVATE_DESTINATIONS", value = "false"
        }) && contains(container.environment, {
        name = "RELAY_DATABASE_CA_REQUIRED", value = "true"
        }) && contains(container.secrets, {
        name      = "RELAY_DATABASE_CA"
        valueFrom = "${aws_secretsmanager_secret.runtime.arn}:database_ca::"
      })
    ])
    error_message = "Both containers require bounded, non-root, read-only, digest-pinned execution."
  }
  assert {
    condition = (
      jsondecode(aws_ecs_task_definition.runtime[0].container_definitions)[0].name == "Main" &&
      jsondecode(aws_ecs_task_definition.runtime[0].container_definitions)[0].portMappings ==
      [{ containerPort = 8080, protocol = "tcp", name = "http", appProtocol = "http" }] &&
      [for secret in jsondecode(
        aws_ecs_task_definition.runtime[0].container_definitions
      )[1].secrets : secret.name] == ["RELAY_DATABASE_CA", "RELAY_DATABASE_URL", "RELAY_DELIVERY_SECRET"]
    )
    error_message = "Express requires Main; the worker must not receive operator or visitor keys."
  }
  assert {
    condition = (
      jsondecode(aws_ecs_task_definition.migration[0].container_definitions)[0].environment == [{
        name = "RELAY_DATABASE_CA_REQUIRED", value = "true"
      }] &&
      jsondecode(aws_ecs_task_definition.migration[0].container_definitions)[0].secrets == [{
        name      = "RELAY_DATABASE_CA"
        valueFrom = "${aws_secretsmanager_secret.migration.arn}:database_ca::"
        }, {
        name      = "RELAY_DATABASE_URL"
        valueFrom = "${aws_secretsmanager_secret.migration.arn}:database_url::"
      }] &&
      (aws_ecs_task_definition.migration[0].task_role_arn == null ||
      aws_ecs_task_definition.migration[0].task_role_arn == "") &&
      alltrue([for container in jsondecode(
        aws_ecs_task_definition.runtime[0].container_definitions
        ) : alltrue([for secret in container.secrets :
          startswith(secret.valueFrom, "${aws_secretsmanager_secret.runtime.arn}:")
      ])])
    )
    error_message = "DDL credentials belong only to the migration task, which has no AWS task role."
  }
  assert {
    condition = (
      jsondecode(aws_iam_role_policy.runtime.policy).Statement == [{
        Effect = "Allow", Resource = aws_sqs_queue.notifications.arn
        Action = [
          "sqs:GetQueueAttributes", "sqs:SendMessage", "sqs:ReceiveMessage", "sqs:DeleteMessage"
        ]
      }] &&
      jsondecode(aws_iam_role.runtime.assume_role_policy).Statement[0].Principal ==
      { Service = "ecs-tasks.amazonaws.com" } &&
      jsondecode(aws_iam_role.runtime.assume_role_policy
      ).Statement[0].Condition.StringEquals["aws:SourceAccount"] == var.aws_account_id
    )
    error_message = "Runtime authority must exclude secret reads, other queues, purge and redrive."
  }
  assert {
    condition = alltrue([for name, policy in aws_iam_role_policy.execution :
      jsondecode(policy.policy).Statement[3].Resource == local.execution_secrets[name] &&
      jsondecode(policy.policy).Statement[3].Action == ["secretsmanager:GetSecretValue"] &&
      jsondecode(policy.policy).Statement[1].Resource == aws_ecr_repository.runtime.arn &&
      jsondecode(policy.policy).Statement[2].Resource ==
      "${aws_cloudwatch_log_group.runtime.arn}:log-stream:*" &&
      length(jsondecode(policy.policy).Statement) == 4
    ])
    error_message = "Execution roles must be restricted to their own secret, registry, and logs."
  }
}

run "reject_mutable_image" {
  command = plan
  variables { runtime_image_digest = "latest" }
  expect_failures = [var.runtime_image_digest]
}
run "reject_short_digest" {
  command = plan
  variables { runtime_image_digest = "sha256:abcdef" }
  expect_failures = [var.runtime_image_digest]
}
run "reject_plaintext_origin" {
  command = plan
  variables { sandbox_origin = "http://relay.example.com" }
  expect_failures = [var.sandbox_origin]
}
run "reject_origin_credentials" {
  command = plan
  variables { sandbox_origin = "https://user:password@relay.example.com" }
  expect_failures = [var.sandbox_origin]
}
run "accept_maximum_dns_length" {
  command = plan
  variables { sandbox_origin = "https://${join(".", [for index in range(127) : "a"])}" }
}
run "reject_excessive_dns_length" {
  command = plan
  variables { sandbox_origin = "https://${join(".", [for index in range(128) : "a"])}" }
  expect_failures = [var.sandbox_origin]
}
run "reject_excessive_label_length" {
  command = plan
  variables { sandbox_origin = "https://${join("", [for index in range(64) : "a"])}.example.com" }
  expect_failures = [var.sandbox_origin]
}
run "reject_empty_dns_label" {
  command = plan
  variables { sandbox_origin = "https://relay..example.com" }
  expect_failures = [var.sandbox_origin]
}
run "reject_origin_path" {
  command = plan
  variables { sandbox_origin = "https://relay.example.com/" }
  expect_failures = [var.sandbox_origin]
}

variable "deploy_service" {
  description = "Runtime gate after image, database, migration and cost verification."
  type        = bool
  default     = false
}

locals {
  express_service_arn = "arn:aws:ecs:us-east-1:${var.aws_account_id}:service/p3-relay/p3-relay"
}

resource "aws_iam_role" "express_infrastructure" {
  name                 = "p3-relay-express-infrastructure"
  max_session_duration = 3600
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow", Action = "sts:AssumeRole"
      Principal = { Service = "ecs.amazonaws.com" }
      Condition = { StringEquals = { "aws:SourceAccount" = var.aws_account_id } }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "express_infrastructure" {
  role = aws_iam_role.express_infrastructure.name
  policy_arn = join("", [
    "arn:aws:iam::aws:policy/service-role/",
    "AmazonECSInfrastructureRoleforExpressGatewayServices"
  ])
}

resource "aws_iam_role" "express_cloudformation" {
  name                 = "p3-relay-express-cloudformation"
  max_session_duration = 3600
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow", Action = "sts:AssumeRole"
      Principal = { Service = "cloudformation.amazonaws.com" }
      Condition = {
        StringEquals = { "aws:SourceAccount" = var.aws_account_id }
        ArnLike = {
          "aws:SourceArn" = join("", [
            "arn:aws:cloudformation:us-east-1:${var.aws_account_id}:",
            "stack/p3-relay-runtime/*"
          ])
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "express_cloudformation" {
  name = "p3-relay-express-service"
  role = aws_iam_role.express_cloudformation.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecs:CreateExpressGatewayService", "ecs:UpdateExpressGatewayService",
          "ecs:DeleteExpressGatewayService", "ecs:DescribeExpressGatewayService",
          "ecs:DescribeServices", "ecs:ListServiceDeployments", "ecs:TagResource"
        ]
        Resource = local.express_service_arn
      },
      {
        Effect = "Allow"
        Action = ["ecs:DescribeServiceDeployments", "ecs:DescribeServiceRevisions"]
        Resource = [
          "arn:aws:ecs:us-east-1:${var.aws_account_id}:service-deployment/p3-relay/p3-relay/*",
          "arn:aws:ecs:us-east-1:${var.aws_account_id}:service-revision/p3-relay/p3-relay/*"
        ]
      },
      {
        Effect = "Allow", Action = "iam:PassRole"
        Resource = [
          aws_iam_role.express_infrastructure.arn,
          aws_iam_role.execution["runtime"].arn,
          aws_iam_role.runtime.arn
        ]
        Condition = {
          StringEquals = {
            "iam:PassedToService" = ["ecs.amazonaws.com", "ecs-tasks.amazonaws.com"]
          }
        }
      }
    ]
  })
}

# Native provider 6.63 cannot pass TaskDefinitionArn. This one-resource stack fills that API gap.
# Keep its service disabled by default; registration of an image alone must never start billing.
resource "aws_cloudformation_stack" "runtime" {
  count              = var.deploy_service ? 1 : 0
  name               = "p3-relay-runtime"
  iam_role_arn       = aws_iam_role.express_cloudformation.arn
  on_failure         = "ROLLBACK"
  timeout_in_minutes = 30
  template_body = jsonencode({
    AWSTemplateFormatVersion = "2010-09-09"
    Resources = {
      Service = {
        Type = "AWS::ECS::ExpressGatewayService"
        Properties = {
          ServiceName           = "p3-relay"
          Cluster               = aws_ecs_cluster.relay.name
          InfrastructureRoleArn = aws_iam_role.express_infrastructure.arn
          TaskDefinitionArn     = aws_ecs_task_definition.runtime[0].arn
          HealthCheckPath       = "/health"
          NetworkConfiguration = {
            # Omit SecurityGroups: Express creates HTTPS ingress and ALB-only task ingress.
            Subnets = [for subnet in aws_subnet.public : subnet.id]
          }
          ScalingTarget = {
            MinTaskCount      = 1, MaxTaskCount = 1
            AutoScalingMetric = "AVERAGE_CPU", AutoScalingTargetValue = 60
          }
          Tags = [
            { Key = "Project", Value = "p3-relay" }, { Key = "ManagedBy", Value = "OpenTofu" }
          ]
        }
      }
    }
    Outputs = {
      Endpoint   = { Value = { "Fn::GetAtt" = ["Service", "Endpoint"] } }
      ServiceArn = { Value = { "Fn::GetAtt" = ["Service", "ServiceArn"] } }
    }
  })
  lifecycle {
    precondition {
      condition     = var.budget_alert_email != ""
      error_message = "Configure the budget alert mailbox before enabling the service."
    }
    precondition {
      condition     = var.runtime_image_digest != ""
      error_message = "Verify and configure an immutable image before enabling the service."
    }
  }
  timeouts {
    create = "40m"
    update = "40m"
    delete = "40m"
  }
  depends_on = [
    aws_budgets_budget.relay,
    aws_iam_role_policy.express_cloudformation,
    aws_iam_role_policy_attachment.express_infrastructure,
    aws_route_table_association.public,
    aws_ecs_cluster_capacity_providers.relay
  ]
}

output "service_endpoint" {
  value = try(aws_cloudformation_stack.runtime[0].outputs["Endpoint"], null)
}

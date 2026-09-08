# Mocked service creation verifies the billable gate and template, not AWS provisioning behavior.
mock_provider "aws" {
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/mock-relay" }
  }
}
variables { aws_account_id = "123456789012" }

run "network_without_compute" {
  command = apply
  assert {
    condition = (
      length(aws_cloudformation_stack.runtime) == 0 &&
      length(aws_subnet.public) == 2 &&
      aws_vpc.relay.cidr_block == "10.33.0.0/24" &&
      length(aws_security_group.migration.ingress) == 0 &&
      aws_ecs_cluster_capacity_providers.relay.capacity_providers == toset(["FARGATE"])
    )
    error_message = "Network preparation must not start compute or expose migration ingress."
  }
  assert {
    condition = (
      aws_subnet.public["a"].availability_zone != aws_subnet.public["b"].availability_zone &&
      alltrue([for subnet in aws_subnet.public : subnet.map_public_ip_on_launch]) &&
      alltrue([for route in aws_route_table.public.route :
        route.cidr_block == "0.0.0.0/0" && route.gateway_id == aws_internet_gateway.relay.id
      ])
    )
    error_message = "Express requires two public AZs; routes must use the IGW, not a billed NAT."
  }
}

run "reject_service_without_image" {
  command = plan
  variables { deploy_service = true }
  expect_failures = [aws_cloudformation_stack.runtime]
}

run "explicit_service_gate" {
  command = apply
  variables {
    budget_alert_email   = "alerts@example.invalid"
    deploy_service       = true
    runtime_image_digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }
  assert {
    condition = (
      length(aws_cloudformation_stack.runtime) == 1 &&
      length(jsondecode(aws_cloudformation_stack.runtime[0].template_body).Resources) == 1 &&
      jsondecode(aws_cloudformation_stack.runtime[0].template_body
      ).Resources.Service.Type == "AWS::ECS::ExpressGatewayService"
    )
    error_message = "The compatibility stack must own only the Express service."
  }
  assert {
    condition = jsondecode(aws_cloudformation_stack.runtime[0].template_body
      ).Resources.Service.Properties == {
      ServiceName           = "p3-relay"
      Cluster               = aws_ecs_cluster.relay.name
      InfrastructureRoleArn = aws_iam_role.express_infrastructure.arn
      TaskDefinitionArn     = aws_ecs_task_definition.runtime[0].arn
      HealthCheckPath       = "/health"
      NetworkConfiguration  = { Subnets = [for subnet in aws_subnet.public : subnet.id] }
      ScalingTarget = {
        MinTaskCount      = 1, MaxTaskCount = 1
        AutoScalingMetric = "AVERAGE_CPU", AutoScalingTargetValue = 60
      }
      Tags = [{ Key = "Project", Value = "p3-relay" }, { Key = "ManagedBy", Value = "OpenTofu" }]
    }
    error_message = "Preserve one task, health checks, owned subnets, and no extra ingress groups."
  }
  assert {
    condition = (
      aws_cloudformation_stack.runtime[0].on_failure == "ROLLBACK" &&
      aws_cloudformation_stack.runtime[0].timeout_in_minutes == 30 &&
      jsondecode(aws_iam_role_policy.express_cloudformation.policy).Statement[0].Resource ==
      local.express_service_arn &&
      jsondecode(aws_iam_role_policy.express_cloudformation.policy).Statement[2].Resource == [
        aws_iam_role.express_infrastructure.arn,
        aws_iam_role.execution["runtime"].arn,
        aws_iam_role.runtime.arn
      ]
    )
    error_message = "Control-plane scope and bounded failure handling must remain explicit."
  }
}

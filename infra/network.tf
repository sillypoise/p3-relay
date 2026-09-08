# Dedicated networking prevents Express Mode from sharing another project's load balancer.
resource "aws_vpc" "relay" {
  cidr_block           = "10.33.0.0/24"
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags                 = { Name = "p3-relay" }
}

resource "aws_internet_gateway" "relay" {
  vpc_id = aws_vpc.relay.id
  tags   = { Name = "p3-relay" }
}

resource "aws_subnet" "public" {
  for_each = {
    a = { zone = "us-east-1a", cidr = "10.33.0.0/26" }
    b = { zone = "us-east-1b", cidr = "10.33.0.64/26" }
  }
  vpc_id                  = aws_vpc.relay.id
  availability_zone       = each.value.zone
  cidr_block              = each.value.cidr
  map_public_ip_on_launch = true
  tags                    = { Name = "p3-relay-${each.key}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.relay.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.relay.id
  }
  tags = { Name = "p3-relay-public" }
}

resource "aws_route_table_association" "public" {
  for_each       = aws_subnet.public
  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

# One-off migrations need outbound access to Railway's assigned TCP proxy port, never ingress.
resource "aws_security_group" "migration" {
  name        = "p3-relay-migration"
  description = "Outbound-only migration task"
  vpc_id      = aws_vpc.relay.id
  ingress     = []
  egress {
    protocol    = "tcp"
    from_port   = 1
    to_port     = 65535
    cidr_blocks = ["0.0.0.0/0"]
    description = "Railway TCP proxy; migration config controls the destination and verified TLS"
  }
}

resource "aws_ecs_cluster" "relay" {
  name = "p3-relay"
  setting {
    name  = "containerInsights"
    value = "disabled"
  }
}

resource "aws_ecs_cluster_capacity_providers" "relay" {
  cluster_name       = aws_ecs_cluster.relay.name
  capacity_providers = ["FARGATE"]
  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    base              = 1
    weight            = 1
  }
}

output "public_subnet_ids" { value = [for subnet in aws_subnet.public : subnet.id] }
output "migration_security_group_id" { value = aws_security_group.migration.id }
output "cluster_name" { value = aws_ecs_cluster.relay.name }

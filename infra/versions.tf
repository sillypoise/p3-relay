terraform {
  required_version = "~> 1.11.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.63.0"
    }
  }
  backend "s3" {}
}

provider "aws" {
  region              = "us-east-1"
  max_retries         = 1
  allowed_account_ids = [var.aws_account_id]
  default_tags {
    tags = { Project = "p3-relay", ManagedBy = "OpenTofu" }
  }
}

variable "aws_account_id" {
  description = "Verified target account; prevents applying to another portfolio account."
  type        = string
  validation {
    condition     = can(regex("^[0-9]{12}$", var.aws_account_id))
    error_message = "Provide the verified 12-digit AWS account ID."
  }
}

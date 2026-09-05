mock_provider "aws" {}
variables { aws_account_id = "123456789012" }
override_resource {
  target = aws_s3_bucket.state
  values = {
    id  = "p3-relay-tofu-state-123456789012"
    arn = "arn:aws:s3:::p3-relay-tofu-state-123456789012"
  }
}

# State protection is verified independently of application resource configuration.
run "private_encrypted_versioned_state" {
  command = plan
  assert {
    condition = (
      aws_s3_bucket_public_access_block.state.block_public_acls &&
      aws_s3_bucket_public_access_block.state.block_public_policy &&
      aws_s3_bucket_public_access_block.state.ignore_public_acls &&
      aws_s3_bucket_public_access_block.state.restrict_public_buckets &&
      aws_s3_bucket.state.force_destroy == false &&
      aws_s3_bucket_versioning.state.versioning_configuration[0].status == "Enabled"
    )
    error_message = "State must remain private, versioned, and protected from forced deletion."
  }
  assert {
    condition = (
      one(aws_s3_bucket_server_side_encryption_configuration.state.rule
      ).apply_server_side_encryption_by_default[0].sse_algorithm == "AES256" &&
      jsondecode(aws_s3_bucket_policy.state.policy
      ).Statement[0].Condition.Bool["aws:SecureTransport"] == "false" &&
      jsondecode(aws_s3_bucket_policy.state.policy).Statement[0].Effect == "Deny"
    )
    error_message = "State requires at-rest encryption and denial of insecure transport."
  }
}

run "reject_empty_account" {
  command = plan
  variables { aws_account_id = "" }
  expect_failures = [var.aws_account_id]
}

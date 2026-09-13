# Railway pulls a public digest without holding AWS credentials. The same manifest must pass
# scanning in Relay's private ECR repository before it is copied here; public ECR has no scanner.
resource "aws_ecrpublic_repository" "gateway" {
  region          = "us-east-1"
  repository_name = "p3-relay-gateway"
  force_destroy   = false

  catalog_data {
    about_text        = "Relay-only PgBouncer gateway for an independent webhook portfolio project."
    description       = "TLS-required PostgreSQL gateway; runtime credentials are never in the image."
    operating_systems = ["Linux"]
    architectures     = ["x86-64"]
  }
}

# Public ECR tags are mutable. Deployment consumers must use the reviewed sha256 manifest digest.
output "gateway_repository_url" {
  value = aws_ecrpublic_repository.gateway.repository_uri
}

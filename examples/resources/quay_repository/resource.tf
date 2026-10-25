resource "quay_organization" "example" {
  name  = "my-org"
  email = "admin@example.com"
}

resource "quay_repository" "example" {
  namespace   = quay_organization.example.name
  name        = "my-app"
  visibility  = "private"
  description = "Container images for my-app"
}

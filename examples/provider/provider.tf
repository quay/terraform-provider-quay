terraform {
  required_providers {
    quay = {
      source  = "quay/quay"
      version = "~> 0.2"
    }
  }
}

provider "quay" {
  # Defaults to https://quay.io. Override for self-hosted instances.
  # Can also be set via the QUAY_URL environment variable.
  url = "https://quay.io"

  # Required. Can also be set via the QUAY_TOKEN environment variable.
  token = var.quay_token
}

variable "quay_token" {
  description = "Quay.io OAuth token"
  type        = string
  sensitive   = true
}

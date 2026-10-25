resource "quay_organization" "example" {
  name  = "my-org"
  email = "admin@example.com"
}

resource "quay_repository" "example" {
  namespace  = quay_organization.example.name
  name       = "my-app"
  visibility = "private"
}

# Webhook triggered on every image push
resource "quay_repository_notification" "push_webhook" {
  repository = quay_repository.example.id
  title      = "Push notification"

  webhook   = { url = "https://hooks.example.com/quay" }
  repo_push = {}
}

# Slack alert for build failures
resource "quay_repository_notification" "build_failure_slack" {
  repository = quay_repository.example.id
  title      = "Build failure alert"

  slack         = { url = "https://hooks.slack.com/services/T00000/B00000/XXXXXXXXXXXX" }
  build_failure = {}
}

# Webhook for High severity (and above) vulnerability findings
resource "quay_repository_notification" "vuln_webhook" {
  repository = quay_repository.example.id
  title      = "High+ vulnerability alert"

  webhook             = { url = "https://hooks.example.com/vulns" }
  vulnerability_found = { level = "High" }
}

# Email notification for main-branch build successes only
resource "quay_repository_notification" "main_build_email" {
  repository = quay_repository.example.id
  title      = "Main branch build succeeded"

  email         = { email = "team@example.com" }
  build_success = { ref_regex = "refs/heads/main" }
}

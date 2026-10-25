package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRepositoryNotification_webhook(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-notif-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/notif-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-notif@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace  = quay_repository.test.namespace
  name       = quay_repository.test.name
  title      = "push webhook"
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "title", "push webhook"),
					resource.TestCheckResourceAttr("quay_repository_notification.test", "webhook.url", "https://httpbin.org/post"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

func TestAccRepositoryNotification_slack(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-slack-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/slack-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-slack@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "slack-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace     = quay_repository.test.namespace
  name          = quay_repository.test.name
  title         = "build failure slack"
  slack         = { url = "https://hooks.slack.com/services/T00000/B00000/XXXXXXXXXXXX" }
  build_failure = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "slack.url", "https://hooks.slack.com/services/T00000/B00000/XXXXXXXXXXXX"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

func TestAccRepositoryNotification_vulnerability(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-vuln-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/vuln-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-vuln@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "vuln-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace           = quay_repository.test.namespace
  name                = quay_repository.test.name
  title               = "vuln alert"
  webhook             = { url = "https://httpbin.org/post" }
  vulnerability_found = { level = "High" }
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "vulnerability_found.level", "High"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

// TestAccRepositoryNotification_vulnerabilityNoLevel verifies that omitting level
// (meaning "any severity") can be created and read back. The provider previously
// required level and errored on read when it was absent.
func TestAccRepositoryNotification_vulnerabilityNoLevel(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-vuln-nolvl-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/vuln-nolvl-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-vuln-nolvl@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "vuln-nolvl-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace           = quay_repository.test.namespace
  name                = quay_repository.test.name
  title               = "any severity"
  webhook             = { url = "https://httpbin.org/post" }
  vulnerability_found = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
					resource.TestCheckNoResourceAttr("quay_repository_notification.test", "vulnerability_found.level"),
				),
			},
		},
	})
}

// TestAccRepositoryNotification_repoImageExpiry verifies that repo_image_expiry
// notifications can be created and read back. Without this event type, importing
// a UI-created notification fails with "Unknown Notification Event".
func TestAccRepositoryNotification_repoImageExpiry(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-expiry-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/expiry-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-expiry@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "expiry-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace         = quay_repository.test.namespace
  name              = quay_repository.test.name
  title             = "image expiry alert"
  webhook           = { url = "https://httpbin.org/post" }
  repo_image_expiry = { days = 7 }
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
					resource.TestCheckResourceAttr("quay_repository_notification.test", "repo_image_expiry.days", "7"),
				),
			},
		},
	})
}

func TestAccRepositoryNotification_buildWithRefRegex(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-build-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/build-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-build@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "build-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace     = quay_repository.test.namespace
  name          = quay_repository.test.name
  title         = "main branch build"
  webhook       = { url = "https://httpbin.org/post" }
  build_success = { ref_regex = "refs/heads/main" }
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository_notification.test", "build_success.ref_regex", "refs/heads/main"),
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
		},
	})
}

package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRepositoryNotification_NoMethodBlock(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"
  name       = "myrepo"
  repo_push  = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

func TestAccRepositoryNotification_MultipleMethodBlocks(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"
  name       = "myrepo"
  webhook    = { url = "https://example.com" }
  slack      = { url = "https://hooks.slack.com/services/T/B/X" }
  repo_push  = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

func TestAccRepositoryNotification_NoEventBlock(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"
  name       = "myrepo"
  webhook    = { url = "https://example.com" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

func TestAccRepositoryNotification_MultipleEventBlocks(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook       = { url = "https://example.com" }
  repo_push     = {}
  build_success = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(exactly one of)`),
			},
		},
	})
}

// TestAccRepositoryNotification_VulnerabilityLevelUnknownStringRejected asserts that
// unrecognised severity strings are rejected at plan time.
func TestAccRepositoryNotification_VulnerabilityLevelUnknownStringRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = "critical" }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// TestAccRepositoryNotification_VulnerabilityLevelIntegerRejected asserts that a bare
// integer is rejected. HCL coerces it to a string that the OneOf validator rejects.
func TestAccRepositoryNotification_VulnerabilityLevelIntegerRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  webhook             = { url = "https://example.com" }
  vulnerability_found = { level = 6 }
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// TestAccRepositoryNotification_QuayNotificationMissingTarget asserts that a
// quay_notification block without a target is rejected at plan time. Quay
// returns 400 "Missing target" at apply time; the schema makes it Required
// to surface the error earlier.
func TestAccRepositoryNotification_QuayNotificationMissingTarget(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "myorg"

  name       = "myrepo"
  quay_notification = {}
  repo_push         = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(required|missing|target)`),
			},
		},
	})
}

// TestAccRepositoryNotification_DeletedOutOfBandRemovesFromState verifies that an
// out-of-band delete is detected on the next refresh. Quay returns 400 (not 404)
// for a missing notification uuid, so Read uses the list endpoint instead.
func TestAccRepositoryNotification_DeletedOutOfBandRemovesFromState(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-notif-oob-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/oob-notif-repo"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-notif-oob@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "oob-notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace  = quay_repository.test.namespace
  name       = quay_repository.test.name
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("quay_repository_notification.test", "uuid"),
				),
			},
			{
				PreConfig: func() {
					c := quayClient()
					ctx := context.Background()
					// Retrieve the uuid from the list endpoint to delete it.
					notifs, err := c.ListRepoNotifications(ctx, orgName+"/oob-notif-repo")
					if err != nil {
						t.Fatalf("listing notifications out-of-band: %v", err)
					}
					for _, n := range notifs {
						if err := c.DeleteRepoNotification(ctx, orgName+"/oob-notif-repo", n.UUID); err != nil {
							t.Fatalf("deleting notification out-of-band: %v", err)
						}
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccRepositoryNotification_RepositoryNotFound(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository_notification" "bad" {
  namespace  = "this-org-does-not-exist-tf-acc"
  name       = "also-missing"
  webhook    = { url = "https://example.com" }
  repo_push  = {}
}
`,
				ExpectError: regexp.MustCompile(`(?i)(not found|404|Error Creating Repository Notification)`),
			},
		},
	})
}

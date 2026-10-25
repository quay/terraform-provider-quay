package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccOrganization_basic(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "email", "tf-acc@example.com"),
				),
			},
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-updated@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "email", "tf-acc-updated@example.com"),
				),
			},
		},
	})
}

func TestAccOrganization_datasource(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-ds-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-ds@example.com"
}

data "quay_organization" "test" {
  name = quay_organization.test.name
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("data.quay_organization.test", "email", "tf-acc-ds@example.com"),
				),
			},
		},
	})
}

// TestAccOrganization_CreateWithInvoiceEmailTrue verifies that invoice_email is persisted
// on create and can be toggled to false. Quay's POST endpoint omits this field,
// so a follow-up PUT must send the configured value, not the API default.
func TestAccOrganization_CreateWithInvoiceEmailTrue(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-inv-email-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name          = %q
  email         = "tf-acc@example.com"
  invoice_email = true
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email", "true"),
				),
			},
			// false must round-trip; an omitted field would be silently ignored.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name          = %q
  email         = "tf-acc@example.com"
  invoice_email = false
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email", "false"),
				),
			},
		},
	})
}

// TestAccOrganization_CreateWithTagExpirationS verifies that tag_expiration_s is persisted
// on create and changed on update. 86400 is chosen because it differs unambiguously
// from Quay's default of 1209600, making a regression immediately visible.
func TestAccOrganization_CreateWithTagExpirationS(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-tag-exp-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name             = %q
  email            = "tf-acc@example.com"
  tag_expiration_s = 86400
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration_s", "86400"),
				),
			},
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name             = %q
  email            = "tf-acc@example.com"
  tag_expiration_s = 604800
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration_s", "604800"),
				),
			},
		},
	})
}

// TestAccOrganization_CreateWithAllOptionalFields verifies that all fields
// excluded from Quay's POST endpoint are persisted when configured simultaneously.
func TestAccOrganization_CreateWithAllOptionalFields(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-all-opts-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name                  = %q
  email                 = "tf-acc@example.com"
  invoice_email         = true
  invoice_email_address = "billing@example.com"
  tag_expiration_s      = 86400
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email", "true"),
					resource.TestCheckResourceAttr("quay_organization.test", "invoice_email_address", "billing@example.com"),
					resource.TestCheckResourceAttr("quay_organization.test", "tag_expiration_s", "86400"),
				),
			},
		},
	})
}

// TestAccOrganization_RemovingEmailClearsIt verifies that removing email from config
// clears it in Quay. UseStateForUnknown on an Optional+Computed field copies the old
// state value into the plan when the attribute is absent, suppressing the update.
func TestAccOrganization_RemovingEmailClearsIt(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-email-clear-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "initial@example.com"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "email", "initial@example.com"),
			},
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name = %q
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "email", ""),
			},
		},
	})
}

// TestAccOrganization_RemovingInvoiceEmailAddressClearsIt verifies that removing
// invoice_email_address from config clears it in Quay.
func TestAccOrganization_RemovingInvoiceEmailAddressClearsIt(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-inv-addr-clear-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name                  = %q
  email                 = "tf-acc@example.com"
  invoice_email_address = "billing@example.com"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "invoice_email_address", "billing@example.com"),
			},
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc@example.com"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "invoice_email_address", ""),
			},
		},
	})
}

func TestAccOrganization_DeleteNonEmpty(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-nodel-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-org@example.com"
}
resource "quay_repository" "child" {
  namespace  = quay_organization.test.name
  name       = "child-repo"
  visibility = "private"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
			},
			// Keeping the repository in config with a hardcoded namespace ensures
			// Terraform has no reason to destroy it before the org.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_repository" "child" {
  namespace  = %q
  name       = "child-repo"
  visibility = "private"
}
`, orgName),
				ExpectError: regexp.MustCompile(`(?i)(non-empty|contains repositories|cannot delete)`),
			},
			// Remove the child out-of-band so CheckDestroy can destroy the org cleanly.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-org@example.com"
}
`, orgName),
				PreConfig: func() {
					if err := quayClient().DeleteRepository(context.Background(), orgName+"/child-repo"); err != nil {
						t.Fatalf("deleting child repository for cleanup: %v", err)
					}
				},
			},
		},
	})
}

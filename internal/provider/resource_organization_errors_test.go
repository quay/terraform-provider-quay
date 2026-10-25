package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/quay/terraform-provider-quay/internal/provider"
)

// TestAccOrganization_TagExpirationSInvalidValueRejected verifies that values outside
// Quay's TAG_EXPIRATION_OPTIONS are rejected at plan time rather than at apply time.
func TestAccOrganization_TagExpirationSInvalidValueRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_organization" "test" {
  name             = "myorg"
  email            = "tf-acc@example.com"
  tag_expiration_s = 999
}
`,
				ExpectError: regexp.MustCompile(`(?i)(one of|invalid attribute value|value must be)`),
			},
		},
	})
}

// orgDataSourceSchema returns the quay_organization data source schema.
func orgDataSourceSchema(t *testing.T) schema.Schema {
	t.Helper()
	ds := provider.NewOrganizationDataSource()
	var resp fwdatasource.SchemaResponse
	ds.Schema(context.Background(), fwdatasource.SchemaRequest{}, &resp)
	return resp.Schema
}

// TestOrgDataSource_IsOrganizationDropped verifies that is_organization is absent from
// the data source schema. Quay never returns this field, so it always read as false.
func TestOrgDataSource_IsOrganizationDropped(t *testing.T) {
	s := orgDataSourceSchema(t)
	if _, ok := s.Attributes["is_organization"]; ok {
		t.Error("is_organization should be removed from the org data source schema — " +
			"Quay never returns this field, so it is always false")
	}
}

// TestOrgDataSource_AdminFieldsAreNullable verifies that admin-only fields are nullable.
// Non-admin tokens omit these fields; null lets callers distinguish absence from zero.
func TestOrgDataSource_AdminFieldsAreNullable(t *testing.T) {
	s := orgDataSourceSchema(t)

	for _, field := range []string{"invoice_email", "invoice_email_address", "tag_expiration_s", "is_free_account"} {
		attr, ok := s.Attributes[field]
		if !ok {
			t.Errorf("expected attribute %q to exist in org data source schema", field)
			continue
		}
		switch a := attr.(type) {
		case schema.BoolAttribute:
			if !a.Optional && !a.Computed {
				t.Errorf("attribute %q must be Optional or Computed to support null values", field)
			}
		case schema.StringAttribute:
			if !a.Optional && !a.Computed {
				t.Errorf("attribute %q must be Optional or Computed to support null values", field)
			}
		case schema.Int64Attribute:
			if !a.Optional && !a.Computed {
				t.Errorf("attribute %q must be Optional or Computed to support null values", field)
			}
		}
	}
}

func TestAccOrganization_DuplicateName(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-dup-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Create the first instance.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "first" {
  name  = %q
  email = "first@example.com"
}
`, orgName),
			},
			// Attempt to create a second resource with the same name — should fail.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "first" {
  name  = %q
  email = "first@example.com"
}

resource "quay_organization" "second" {
  name  = %q
  email = "second@example.com"
}
`, orgName, orgName),
				ExpectError: regexp.MustCompile(`(?i)(already exists|conflict|409|Error Creating Organization)`),
			},
		},
	})
}

func TestAccOrganization_DataSourceNotFound(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
data "quay_organization" "missing" {
  name = "this-org-does-not-exist-tf-acc"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(not found|404|Error Reading Organization)`),
			},
		},
	})
}

// TestAccOrganization_PostCreateUpdateFailureOrphansOrg verifies that when the
// post-create PUT fails, the org is still saved to state. Without this, the org
// exists in Quay but Terraform treats it as new and the next apply fails with
// "already exists".
func TestAccOrganization_PostCreateUpdateFailureOrphansOrg(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-orphan-org")

	// Delete the org even if it was orphaned without any Terraform state entry.
	t.Cleanup(func() {
		_ = quayClient().DeleteOrganization(context.Background(), orgName)
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Step 1: tag_expiration_s=999 is invalid; the POST succeeds but
			// the follow-up PUT fails. State must still be saved.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "orphan" {
  name             = %q
  email            = "orphan@example.com"
  tag_expiration_s = 999
}
`, orgName),
				ExpectError: regexp.MustCompile(`(?i)(Error Configuring Organization|tag_expiration_s|400|invalid)`),
			},
			// Step 2: re-apply with a valid config. If Step 1 saved state, this
			// succeeds; otherwise Quay returns "already exists".
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "orphan" {
  name  = %q
  email = "orphan@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.orphan", "name", orgName),
					resource.TestCheckResourceAttr("quay_organization.orphan", "email", "orphan@example.com"),
				),
			},
		},
	})
}

// TestAccOrganization_DeletedOutOfBand verifies that an out-of-band delete is
// detected on the next refresh and produces a non-empty plan.
func TestAccOrganization_DeletedOutOfBand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-oob-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "oob" {
  name  = %q
  email = "oob@example.com"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.oob", "name", orgName),
				),
			},
			{
				PreConfig: func() {
					if err := quayClient().DeleteOrganization(t.Context(), orgName); err != nil {
						t.Fatalf("deleting organization out-of-band: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

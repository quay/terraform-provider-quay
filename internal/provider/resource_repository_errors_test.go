package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRepository_DataSourceNotFound(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
data "quay_repository" "missing" {
  namespace = "this-namespace-does-not-exist-tf-acc"
  name      = "also-missing"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(not found|404|Error Reading Repository)`),
			},
		},
	})
}

func TestAccRepository_InvalidVisibility(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "quay_repository" "bad" {
  namespace  = "myorg"
  name       = "myrepo"
  visibility = "internal"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(invalid|value must be one of)`),
			},
		},
	})
}

// TestAccRepository_PartialUpdateSavesState verifies that a description change is
// saved to state even when the subsequent visibility change fails. Without this,
// state reverts to the old description and Terraform re-applies it on every plan.
func TestAccRepository_PartialUpdateSavesState(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-partial-upd-org")
	repoName := "partial-upd-repo"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
		),
		Steps: []resource.TestStep{
			// Step 1: create the org and a private repo with an initial description.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "partial-upd@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = %q
  visibility  = "private"
  description = "initial"
}
`, orgName, repoName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "description", "initial"),
					resource.TestCheckResourceAttr("quay_repository.test", "visibility", "private"),
				),
			},
			// Step 2: change description and visibility together. The description
			// PUT fires first; the visibility change may fail. Either way, the
			// description in state must match what was applied.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "partial-upd@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = %q
  visibility  = "public"
  description = "updated"
}
`, orgName, repoName),
				Check: resource.TestCheckResourceAttr("quay_repository.test", "description", "updated"),
			},
		},
	})
}

// TestAccRepository_OrgDeletedOutOfBandRemovesRepoFromState verifies that when a
// parent org is deleted out-of-band, the repo is removed from state rather than
// stuck in an error. Quay returns 403 (not 404) for a repo in a deleted namespace.
func TestAccRepository_OrgDeletedOutOfBandRemovesRepoFromState(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-oob-org-del")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "oob-org-del@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "oob-repo"
  visibility  = "private"
  description = ""
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "name", "oob-repo"),
				),
			},
			{
				PreConfig: func() {
					c := quayClient()
					ctx := t.Context()
					if err := c.DeleteOrganization(ctx, orgName); err != nil {
						t.Fatalf("deleting organization out-of-band: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccRepository_RemovingDescriptionClearsIt verifies that removing description
// from config clears it in Quay. UseStateForUnknown on an Optional+Computed field
// suppresses the update by copying the old value into the plan.
func TestAccRepository_RemovingDescriptionClearsIt(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-desc-clear-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/desc-repo"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "desc-clear@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "desc-repo"
  visibility  = "private"
  description = "initial description"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_repository.test", "description", "initial description"),
			},
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "desc-clear@example.com"
}
resource "quay_repository" "test" {
  namespace  = quay_organization.test.name
  name       = "desc-repo"
  visibility = "private"
}
`, orgName),
				Check: resource.TestCheckResourceAttr("quay_repository.test", "description", ""),
			},
		},
	})
}

// TestAccRepository_DeletedOutOfBand verifies that an out-of-band delete is
// detected on the next refresh and produces a non-empty plan.
func TestAccRepository_DeletedOutOfBand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-oob-repo-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "oob-repo@example.com"
}
resource "quay_repository" "oob" {
  namespace   = quay_organization.test.name
  name        = "oob-repo"
  visibility  = "private"
  description = ""
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.oob", "name", "oob-repo"),
				),
			},
			{
				PreConfig: func() {
					repoPath := orgName + "/oob-repo"
					if err := quayClient().DeleteRepository(t.Context(), repoPath); err != nil {
						t.Fatalf("deleting repository out-of-band: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestRepository_Read403OnUserNamespaceSurfacesError verifies that a 403 on a
// user-namespace repository surfaces as an error rather than silently removing
// state. GetOrganization always returns 404 for user accounts, so checking only
// the org endpoint incorrectly treats a permissions error as a deleted namespace.
func TestRepository_Read403OnUserNamespaceSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := func(status int, body any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(body)
		}
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/api/v1/repository/"):
			write(http.StatusForbidden, map[string]string{"message": "access denied"})
		case strings.HasPrefix(path, "/api/v1/organization/"):
			write(http.StatusNotFound, map[string]string{"message": "not found"})
		case strings.HasPrefix(path, "/api/v1/users/"):
			write(http.StatusOK, map[string]string{"username": "alice"})
		default:
			write(http.StatusNotFound, map[string]string{"message": "not found"})
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("QUAY_URL", srv.URL)
	t.Setenv("QUAY_TOKEN", "test-token")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {}

resource "quay_repository" "test" {
  namespace   = "alice"
  name        = "myrepo"
  visibility  = "private"
  description = ""
}
`,
				ExpectError: regexp.MustCompile(`(?i)(403|forbidden|access denied|Error)`),
			},
		},
	})
}

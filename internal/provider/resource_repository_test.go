package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccRepository_basic(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-repo-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/my-repo"),
		),
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-repo@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "my-repo"
  visibility  = "private"
  description = "acceptance test repo"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "name", "my-repo"),
					resource.TestCheckResourceAttr("quay_repository.test", "visibility", "private"),
					resource.TestCheckResourceAttr("quay_repository.test", "description", "acceptance test repo"),
					resource.TestCheckResourceAttr("quay_repository.test", "id", orgName+"/my-repo"),
				),
			},
			// Update description
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-repo@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "my-repo"
  visibility  = "private"
  description = "updated description"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "description", "updated description"),
				),
			},
			// Change visibility
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-repo@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "my-repo"
  visibility  = "public"
  description = "updated description"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "visibility", "public"),
				),
			},
		},
	})
}

func TestAccRepository_datasource(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-ds-repo-org")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/ds-repo"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-ds-repo@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "ds-repo"
  visibility  = "private"
  description = "datasource test"
}
data "quay_repository" "test" {
  namespace = quay_repository.test.namespace
  name      = quay_repository.test.name
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.quay_repository.test", "visibility", "private"),
					resource.TestCheckResourceAttr("data.quay_repository.test", "description", "datasource test"),
				),
			},
		},
	})
}

// TestAccRepository_CreatePublic verifies that public visibility is reflected in state.
// Quay's POST /repository response omits is_public, so the actual visibility must be fetched.
func TestAccRepository_CreatePublic(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-pub-repo-org")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/pub-repo"),
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-pub-repo@example.com"
}
resource "quay_repository" "test" {
  namespace  = quay_organization.test.name
  name       = "pub-repo"
  visibility = "public"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "visibility", "public"),
				),
			},
		},
	})
}

func TestAccRepository_DeleteNonEmpty(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-nodel-repo-org")
	repoPath := orgName + "/nodel-repo"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkRepositoryDestroyed(repoPath),
			checkOrganizationDestroyed(orgName),
		),
		Steps: []resource.TestStep{
			// Push a real tag so the repository is non-empty when destroy runs.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-repo@example.com"
}
resource "quay_repository" "test" {
  namespace  = quay_organization.test.name
  name       = "nodel-repo"
  visibility = "private"
}
`, orgName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_repository.test", "name", "nodel-repo"),
					func(_ *terraform.State) error {
						pushImageToQuay(t, repoPath, "latest")
						return nil
					},
				),
			},
			// The repository has a tag; the provider must refuse to destroy it.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-repo@example.com"
}
`, orgName),
				ExpectError: regexp.MustCompile(`(?i)(non-empty|contains active tags|cannot delete)`),
			},
			// Delete the tag out-of-band so Terraform can destroy the now-empty repository.
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "tf-acc-nodel-repo@example.com"
}
resource "quay_repository" "test" {
  namespace  = quay_organization.test.name
  name       = "nodel-repo"
  visibility = "private"
}
`, orgName),
				PreConfig: func() {
					if err := quayClient().DeleteTag(context.Background(), repoPath, "latest"); err != nil {
						t.Fatalf("deleting tag for cleanup: %v", err)
					}
				},
			},
		},
	})
}

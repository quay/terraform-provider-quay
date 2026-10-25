package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// notificationImportID builds the import ID from state attributes.
func notificationImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		r, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %q not found in state", resourceName)
		}
		return fmt.Sprintf("%s/%s/%s",
			r.Primary.Attributes["namespace"],
			r.Primary.Attributes["name"],
			r.Primary.Attributes["uuid"],
		), nil
	}
}

// ---------------------------------------------------------------------------
// quay_organization
// ---------------------------------------------------------------------------

func TestAccOrganization_importCommand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-org-cmd")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
				),
			},
			{
				Config:            cfg,
				ResourceName:      "quay_organization.test",
				ImportState:       true,
				ImportStateId:     orgName,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccOrganization_importBlock(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-org-blk")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
				),
			},
			{
				Config:          cfg,
				ResourceName:    "quay_organization.test",
				ImportStateId:   orgName,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
			},
		},
	})
}

func TestAccOrganization_generateConfig(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-org-gen")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy:             checkOrganizationDestroyed(orgName),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("quay_organization.test", "name", orgName),
				),
			},
			{
				Config:          cfg,
				ResourceName:    "quay_organization.test",
				ImportStateId:   orgName,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// quay_repository
// ---------------------------------------------------------------------------

func TestAccRepository_importCommand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-repo-cmd")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-repo"
  visibility  = "private"
  description = "import test"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-repo"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:            cfg,
				ResourceName:      "quay_repository.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccRepository_importBlock(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-repo-blk")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-repo"
  visibility  = "private"
  description = "import test"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-repo"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:          cfg,
				ResourceName:    "quay_repository.test",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
			},
		},
	})
}

func TestAccRepository_generateConfig(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-repo-gen")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-repo"
  visibility  = "private"
  description = "import test"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-repo"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:          cfg,
				ResourceName:    "quay_repository.test",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// quay_repository_notification
// ---------------------------------------------------------------------------

func TestAccRepositoryNotification_importCommand(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-notif-cmd")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace  = quay_repository.test.namespace
  name       = quay_repository.test.name
  title      = "import test"
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-notif-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:            cfg,
				ResourceName:      "quay_repository_notification.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: notificationImportID("quay_repository_notification.test"),
			},
		},
	})
}

func TestAccRepositoryNotification_importBlock(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-notif-blk")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace  = quay_repository.test.namespace
  name       = quay_repository.test.name
  title      = "import test"
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-notif-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:            cfg,
				ResourceName:      "quay_repository_notification.test",
				ImportState:       true,
				ImportStateKind:   resource.ImportBlockWithID,
				ImportStateIdFunc: notificationImportID("quay_repository_notification.test"),
			},
		},
	})
}

// TestAccRepositoryNotification_generateConfig is absent because GenerateConfig
// redacts Sensitive attributes, omitting Required fields like webhook.url and
// causing the subsequent plan to fail.

// quay_repository requires a "namespace/name" import ID; malformed IDs must be rejected.

func TestAccRepository_importInvalidID_noSeparator(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-repo-bad1")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-repo"
  visibility  = "private"
  description = "import error test"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-repo"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:        cfg,
				ResourceName:  "quay_repository.test",
				ImportState:   true,
				ImportStateId: "no-slash-at-all",
				ExpectError:   regexp.MustCompile(`(?i)(invalid import id|expected 2 parts)`),
			},
		},
	})
}

func TestAccRepository_importInvalidID_emptyNamespace(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-repo-bad2")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-repo"
  visibility  = "private"
  description = "import error test"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-repo"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:        cfg,
				ResourceName:  "quay_repository.test",
				ImportState:   true,
				ImportStateId: "/import-repo",
				ExpectError:   regexp.MustCompile(`(?i)(invalid import id|must not be empty)`),
			},
		},
	})
}

func TestAccRepository_importInvalidID_emptyName(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-repo-bad3")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-repo"
  visibility  = "private"
  description = "import error test"
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-repo"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:        cfg,
				ResourceName:  "quay_repository.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s/", orgName),
				ExpectError:   regexp.MustCompile(`(?i)(invalid import id|must not be empty)`),
			},
		},
	})
}

func TestAccRepositoryNotification_importInvalidID_twoPartsOnly(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-notif-bad1")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace  = quay_repository.test.namespace
  name       = quay_repository.test.name
  title      = "import error test"
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-notif-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:        cfg,
				ResourceName:  "quay_repository_notification.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s/import-notif-repo", orgName),
				ExpectError:   regexp.MustCompile(`(?i)(invalid import id|expected 3 parts)`),
			},
		},
	})
}

func TestAccRepositoryNotification_importInvalidID_emptyUUID(t *testing.T) {
	skipIfNoAcc(t)
	orgName := rname("tf-acc-import-notif-bad2")
	cfg := providerConfig() + fmt.Sprintf(`
resource "quay_organization" "test" {
  name  = %q
  email = "import@example.com"
}
resource "quay_repository" "test" {
  namespace   = quay_organization.test.name
  name        = "import-notif-repo"
  visibility  = "private"
  description = ""
}
resource "quay_repository_notification" "test" {
  namespace  = quay_repository.test.namespace
  name       = quay_repository.test.name
  title      = "import error test"
  webhook    = { url = "https://httpbin.org/post" }
  repo_push  = {}
}
`, orgName)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkOrganizationDestroyed(orgName),
			checkRepositoryDestroyed(orgName+"/import-notif-repo"),
			checkNotificationDestroyed("quay_repository_notification.test"),
		),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:        cfg,
				ResourceName:  "quay_repository_notification.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s/import-notif-repo/", orgName),
				ExpectError:   regexp.MustCompile(`(?i)(invalid import id|must not be empty)`),
			},
		},
	})
}

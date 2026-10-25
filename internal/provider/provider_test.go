package provider_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/quay/terraform-provider-quay/internal/client"
	"github.com/quay/terraform-provider-quay/internal/provider"
)

// testState holds the Quay instance URL and token shared across all acceptance tests.
var testState struct {
	quayURL   string
	quayToken string
	// runID prevents name collisions between parallel matrix jobs.
	runID string
}

// rname returns a resource name unique to this test run to prevent cross-run collisions.
func rname(base string) string {
	return fmt.Sprintf("%s-%s", base, testState.runID)
}

// TestMain starts a Quay testcontainer once per package run when TF_ACC is set.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests contains the real TestMain logic. os.Exit is kept in TestMain so defers here unwind normally.
func runTests(m *testing.M) int {
	if os.Getenv("TF_ACC") == "" {
		return m.Run()
	}

	// Allow overriding with a pre-existing Quay instance.
	if u := os.Getenv("QUAY_URL"); u != "" {
		testState.quayURL = u
		testState.quayToken = os.Getenv("QUAY_TOKEN")
		testState.runID = fmt.Sprintf("%06x", rand.Intn(0xffffff))
		return m.Run()
	}

	// Ryuk doesn't work under Podman rootless; containers must be cleaned up explicitly.
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true") //nolint:errcheck

	ctx := context.Background()
	quayC, err := runQuay(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: could not start Quay testcontainer: %v\n", err)
		return 1
	}
	defer quayC.Cleanup(ctx)

	testState.quayURL = quayC.baseURL
	testState.quayToken = quayC.accessToken
	testState.runID = fmt.Sprintf("%06x", rand.Intn(0xffffff))

	if err := os.Setenv("QUAY_URL", testState.quayURL); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: could not set QUAY_URL: %v\n", err)
		return 1
	}
	if err := os.Setenv("QUAY_TOKEN", testState.quayToken); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: could not set QUAY_TOKEN: %v\n", err)
		return 1
	}

	return m.Run()
}

// providerFactories returns the provider factory map for resource.Test.
func providerFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"quay": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// providerConfig returns an HCL provider block for the shared test container.
func providerConfig() string {
	return fmt.Sprintf(`
provider "quay" {
  url   = %q
  token = %q
}
`, testState.quayURL, testState.quayToken)
}

// skipIfNoAcc skips acceptance tests when TF_ACC is not set.
func skipIfNoAcc(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set — skipping acceptance test")
	}
}

// quayClient returns a Quay API client pointed at the shared test instance.
func quayClient() *client.Client {
	return client.New(testState.quayURL, testState.quayToken, client.DefaultTimeout)
}

// checkOrganizationDestroyed fails if the named organization still exists after destroy.
func checkOrganizationDestroyed(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		_, err := quayClient().GetOrganization(context.Background(), name)
		if client.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking organization %q: %w", name, err)
		}
		return fmt.Errorf("organization %q still exists after destroy", name)
	}
}

// checkRepositoryDestroyed fails if the named repository still exists after destroy.
func checkRepositoryDestroyed(repository string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		_, err := quayClient().GetRepository(context.Background(), repository)
		if client.IsNotFound(err) {
			return nil
		}
		// A 403 means the org namespace was removed or the token lacks scope.
		if client.IsForbidden(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking repository %q: %w", repository, err)
		}
		return fmt.Errorf("repository %q still exists after destroy", repository)
	}
}

// checkNotificationDestroyed fails if the notification still exists after destroy.
func checkNotificationDestroyed(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return nil
		}
		namespace := rs.Primary.Attributes["namespace"]
		name := rs.Primary.Attributes["name"]
		repository := namespace + "/" + name
		uuid := rs.Primary.Attributes["uuid"]
		_, err := quayClient().GetRepoNotification(context.Background(), repository, uuid)
		if client.IsNotFound(err) {
			return nil
		}
		// A 403 means the org/repo was removed or the token lacks scope.
		if client.IsForbidden(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking notification %q on %q: %w", uuid, repository, err)
		}
		return fmt.Errorf("notification %q on repository %q still exists after destroy", uuid, repository)
	}
}

func TestProvider_Configure(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{Config: providerConfig()},
		},
	})
}

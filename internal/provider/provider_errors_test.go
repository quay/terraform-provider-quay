package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestProvider_TokenFromEnv verifies that QUAY_TOKEN can substitute for the token attribute.
func TestProvider_TokenFromEnv(t *testing.T) {
	skipIfNoAcc(t)

	t.Setenv("QUAY_TOKEN", testState.quayToken)
	t.Setenv("QUAY_URL", testState.quayURL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{Config: `provider "quay" {}`},
		},
	})
}

// TestProvider_TokenInBlockURLFromEnv verifies that QUAY_URL can substitute for the url attribute.
func TestProvider_TokenInBlockURLFromEnv(t *testing.T) {
	skipIfNoAcc(t)

	t.Setenv("QUAY_URL", testState.quayURL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  token = "` + testState.quayToken + `"
}
`,
			},
		},
	})
}

// TestProvider_URLInBlockTokenFromEnv verifies that QUAY_TOKEN can substitute for the token attribute.
func TestProvider_URLInBlockTokenFromEnv(t *testing.T) {
	skipIfNoAcc(t)

	t.Setenv("QUAY_TOKEN", testState.quayToken)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url = "` + testState.quayURL + `"
}
`,
			},
		},
	})
}

// TestProvider_UnknownToken verifies that an unknown token is rejected at configure time.
// A silent fallback to QUAY_TOKEN could send the token to the wrong host or drop resources from state.
func TestProvider_UnknownToken(t *testing.T) {
	t.Setenv("QUAY_TOKEN", "env-token-should-not-be-used")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "test" {
  input = "not-yet-known"
}

provider "quay" {
  url   = "https://quay.io"
  token = terraform_data.test.output
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(unknown|Invalid Quay API Token)`),
			},
		},
	})
}

// TestProvider_UnknownURL verifies that an unknown url is rejected at configure time.
// A silent fallback to QUAY_URL could send requests to the wrong host.
func TestProvider_UnknownURL(t *testing.T) {
	t.Setenv("QUAY_TOKEN", "test-token")
	t.Setenv("QUAY_URL", "https://should-not-be-used.example.com")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "test" {
  input = "https://not-yet-known.example.com"
}

provider "quay" {
  url   = terraform_data.test.output
  token = "test-token"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(unknown|Invalid Quay URL)`),
			},
		},
	})
}

// TestProvider_HTTPURLRejected verifies that http:// URLs are rejected to prevent the token leaking in plaintext.
func TestProvider_HTTPURLRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url   = "http://quay.example.com"
  token = "test-token"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(http.*plaintext|Invalid Quay URL)`),
			},
		},
	})
}

// TestProvider_NegativeTimeoutInConfig verifies that a negative timeout is rejected at configure time.
// A negative time.Duration is treated by http.Client as no timeout, silently ignoring the user's intent.
func TestProvider_NegativeTimeoutInConfig(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url     = "https://quay.io"
  token   = "test-token"
  timeout = -1
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(negative|invalid.*timeout|timeout.*invalid)`),
			},
		},
	})
}

// TestProvider_NegativeTimeoutInEnv verifies that a negative QUAY_TIMEOUT is rejected at configure time.
func TestProvider_NegativeTimeoutInEnv(t *testing.T) {
	t.Setenv("QUAY_TIMEOUT", "-1")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url   = "https://quay.io"
  token = "test-token"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(negative|invalid.*timeout|timeout.*invalid)`),
			},
		},
	})
}

// TestProvider_MissingToken verifies the provider errors when no token is supplied.
func TestProvider_MissingToken(t *testing.T) {
	t.Setenv("QUAY_TOKEN", "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url = "https://quay.io"
  # token deliberately omitted
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`Missing Quay API Token`),
			},
		},
	})
}

// TestProvider_InvalidToken verifies that an invalid token surfaces a meaningful API error.
func TestProvider_InvalidToken(t *testing.T) {
	skipIfNoAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "quay" {
  url   = "` + testState.quayURL + `"
  token = "this-token-is-completely-invalid"
}

data "quay_repository" "test" {
  namespace = "myorg"
  name      = "myrepo"
}
`,
				ExpectError: regexp.MustCompile(`(?i)(401|Unauthorized|invalid_token|Error Reading Repository)`),
			},
		},
	})
}

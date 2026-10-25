# Terraform Provider for Quay

A Terraform provider for managing resources on [Quay.io](https://quay.io) or a
self-hosted [Red Hat Quay](https://www.redhat.com/en/technologies/cloud-computing/quay)
instance.

[![Build](https://github.com/quay/terraform-provider-quay/actions/workflows/ci.yml/badge.svg)](https://github.com/quay/terraform-provider-quay/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/release/quay/terraform-provider-quay.svg?style=flat)](https://github.com/quay/terraform-provider-quay/releases/latest)
[![Repository](https://img.shields.io/badge/github-repo-blue?logo=github)](https://github.com/quay/terraform-provider-quay)
[![Registry](https://img.shields.io/badge/Terraform%20Registry-quay%2Fquay-623CE4)](https://registry.terraform.io/providers/quay/quay/latest)
![Known Vulnerabilities](https://snyk.io/test/github/quay/terraform-provider-quay/badge.svg)
[![Conventional Commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=flat)](https://conventionalcommits.org)

## Documentation

Full provider documentation, including argument references and import examples,
is available on the [Terraform Registry](https://registry.terraform.io/providers/quay/quay/latest/docs).

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.5
- [Go](https://golang.org/doc/install) >= 1.27 (for development only)

## Usage

```hcl
terraform {
  required_providers {
    quay = {
      source  = "quay/quay"
      version = "~> 0.2"
    }
  }
}

provider "quay" {
  # Defaults to https://quay.io. Override for self-hosted instances.
  # Can also be set via the QUAY_URL environment variable.
  url = "https://quay.io"

  # Can also be set via the QUAY_TOKEN environment variable.
  token = var.quay_token
}
```

## Supported Resources

| Resource | Description |
|---|---|
| [`quay_organization`](https://registry.terraform.io/providers/quay/quay/latest/docs/resources/organization) | Manages a Quay organization |
| [`quay_repository`](https://registry.terraform.io/providers/quay/quay/latest/docs/resources/repository) | Manages a Quay repository |
| [`quay_repository_notification`](https://registry.terraform.io/providers/quay/quay/latest/docs/resources/repository_notification) | Manages a notification on a repository |

## Supported Data Sources

| Data Source | Description |
|---|---|
| [`quay_organization`](https://registry.terraform.io/providers/quay/quay/latest/docs/data-sources/organization) | Reads information about a Quay organization |
| [`quay_repository`](https://registry.terraform.io/providers/quay/quay/latest/docs/data-sources/repository) | Reads information about a Quay repository |

---

## Development

### Requirements

- [Go](https://golang.org/doc/install) >= 1.27
- [Podman](https://podman.io/) or [Docker](https://www.docker.com/) (for acceptance tests)

### Building

```shell
make build
```

### Installing Locally

Installs the provider binary into `~/.terraform.d/plugins/` so you can use it
with a local Terraform configuration:

```shell
make install
```

### Running Unit Tests

No external dependencies required:

```shell
make test
```

### Running Acceptance Tests

Acceptance tests spin up a real Quay instance automatically using
[testcontainers-go](https://golang.testcontainers.org/) — no manual setup
needed if you have Docker or Podman available:

```shell
make testacc
```

To run against an existing Quay instance instead (skips the testcontainer):

```shell
export QUAY_URL=https://quay.io   # or your self-hosted URL
export QUAY_TOKEN=your_token_here
make testacc
```

### Regenerating Documentation

Documentation in `docs/` is generated from schema descriptions in the provider
code and example files in `examples/`. After changing either, run:

```shell
make generate
```

---

## Publishing to the Terraform Registry

### One-time setup

#### 1. Generate a GPG signing key (RSA — the Registry rejects ECC keys)

```shell
gpg --full-generate-key
# Choose: (1) RSA and RSA, 4096 bits
```

#### 2. Upload the public key to the Terraform Registry

```shell
gpg --armor --export your-email@example.com
```

Paste the output at **https://registry.terraform.io/settings/gpg-keys**.

#### 3. Add secrets to your GitHub repository

Go to **Settings → Secrets and variables → Actions** and add:

| Secret | Value |
|---|---|
| `GPG_PRIVATE_KEY` | Output of `gpg --armor --export-secret-keys YOUR_KEY_ID` |
| `PASSPHRASE` | The passphrase set when generating the key |

#### 4. Register the provider on the Terraform Registry

1. Sign in to [registry.terraform.io](https://registry.terraform.io) with GitHub.
2. Click **Publish → Provider** in the top-right navigation.
3. Select your GitHub namespace and the `terraform-provider-quay` repository.
4. Accept the Terms of Use and click **Publish Provider**.

### Releasing a new version

Releases are fully automated. Merge a conventional-commit PR to `main` and
release-please will open a Release PR with the computed version and changelog.
Merging the Release PR creates the tag and triggers the `cd.yml` workflow, which
builds binaries for all platforms, signs the checksums with your GPG key, and
creates a GitHub Release. The Registry webhook ingests it within a few minutes.

See [CONTRIBUTING.md](CONTRIBUTING.md#releasing) for the full release process.

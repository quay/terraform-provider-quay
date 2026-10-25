# Contributing

Thank you for taking the time to contribute. This document covers everything
you need to get a development environment running, make a change, and get it
merged.

## Table of contents

- [Development environment](#development-environment)
- [Project layout](#project-layout)
- [Making changes](#making-changes)
- [Testing](#testing)
- [Documentation](#documentation)
- [Commit messages and pull requests](#commit-messages-and-pull-requests)
- [Releasing](#releasing)

---

## Development environment

### Option A — Nix (recommended)

The repository ships a `flake.nix` that provides a fully-pinned shell with all
required tools. If you have [Nix](https://nixos.org/download/) and
[direnv](https://direnv.net/) installed, the shell activates automatically when
you enter the project directory:

```shell
direnv allow
```

To enter the shell manually without direnv:

```shell
nix develop
```

The shell provides: `go`, `golangci-lint`, `goimports`, `gopls`, `terraform`,
`goreleaser`, and `git`.

### Option B — manual

Install these tools yourself at the versions shown:

| Tool | Minimum version | Notes |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.27 | Declared in `go.mod` |
| [Terraform CLI](https://developer.hashicorp.com/terraform/install) | 1.5 | Needed for `go generate` and acceptance tests |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | 2.x | `make lint` |
| [Podman](https://podman.io/) or [Docker](https://www.docker.com/) | any recent | Acceptance tests spin up a Quay container |

---

## Project layout

```
.
├── internal/
│   ├── client/           # HTTP client for the Quay REST API
│   └── provider/         # Terraform plugin framework resources and data sources
├── examples/             # HCL examples embedded in generated documentation
├── templates/            # tfplugindocs templates (index page only)
├── docs/                 # Generated documentation — do not edit by hand
├── .github/workflows/    # CI: tests, release, PR lint
├── flake.nix             # Nix dev shell
├── GNUmakefile           # Common development targets
├── main.go               # Provider entry point and go:generate directive
└── .goreleaser.yml       # Multi-platform release configuration
```

The two packages under `internal/` have a clear boundary:

- **`client`** — a plain Go HTTP client. It knows nothing about Terraform. Tests
  here use `net/http/httptest` and need no external services.
- **`provider`** — the Terraform Plugin Framework layer. It translates between
  Terraform state and the client. Tests here are acceptance tests that require a
  live Quay instance (provided automatically by testcontainers).

---

## Pre-commit hooks

The repository ships a `.pre-commit-config.yaml` with hooks for formatting,
vetting, module hygiene, and linting. The Go hooks use `language: golang` so
pre-commit manages its own isolated tool environments — no dependency on the
Nix shell or any particular system installation.

Install the hooks once after cloning:

```shell
pre-commit install --install-hooks
```

Run all hooks manually at any time:

```shell
pre-commit run --all-files
```

You need [pre-commit](https://pre-commit.com/#install) and `go` on your
`PATH`. Everything else (`goimports`, `golangci-lint`) is fetched and cached
by pre-commit automatically.

---

## Making changes

### Adding a resource or data source

1. Add the client methods to `internal/client/` with a corresponding `_test.go`
   using `httptest.NewServer`.
2. Implement the resource or data source in `internal/provider/`, following the
   patterns in the existing files.
3. Register it in `internal/provider/provider.go`.
4. Add an example HCL file under `examples/resources/<name>/resource.tf` or
   `examples/data-sources/<name>/data-source.tf`.
5. Regenerate documentation (see [Documentation](#documentation)).
6. Write acceptance tests.

### Changing an existing resource schema

Schema changes that remove or rename attributes are breaking changes and require
a state migration. If you are unsure whether your change is breaking, open a
discussion before investing significant effort.

---

## Testing

### Unit tests

No external dependencies required. These cover the HTTP client and any
provider-level tests that do not require `TF_ACC`:

```shell
make test
```

### Acceptance tests

Acceptance tests require a container runtime (Podman or Docker). `TestMain`
starts a Quay container automatically via
[testcontainers-go](https://golang.testcontainers.org/) — no manual setup
needed:

```shell
make testacc
```

To run against an existing Quay instance instead (faster iteration):

```shell
export QUAY_URL=http://your-quay-instance:8080
export QUAY_TOKEN=your_token_here
make testacc
```

To run a single test:

```shell
make testacc TESTARGS="-run TestAccOrganization_basic"
```

### What to test

Every new resource or data source should have:

- **Client tests** — happy path CRUD and the common error cases (404, 409, 401)
  using a mock HTTP server.
- **Acceptance tests** — at minimum a basic create/read/update/delete cycle and
  an import test. Error cases that can be caught at plan time (invalid attribute
  values, schema validation) can use `resource.UnitTest` and do not need a live
  Quay instance.

### CI pipeline

| Job | Tooling |
|---|---|
| Schema validation | `pipx run pre-commit` |
| Build, lint | `actions/setup-go` + `golangci/golangci-lint-action` |
| Docs drift | `actions/setup-go` + `hashicorp/setup-terraform` |
| Unit tests | `actions/setup-go` |
| Acceptance tests | `actions/setup-go` + `hashicorp/setup-terraform` (matrix of Terraform CLI versions) |
| Release | `actions/setup-go` + `goreleaser/goreleaser-action` |

All CI jobs use `actions/setup-go` pinned to the version declared in `go.mod`.
The Nix dev shell is available locally (see [Option A](#option-a--nix-recommended)
above) but is not used in CI.

---

## Documentation

Documentation in `docs/` is generated from three sources:

- **Schema descriptions** — the `Description` fields on each attribute in the
  resource/data source `Schema()` method.
- **Example files** — HCL under `examples/`.
- **Templates** — Markdown templates under `templates/` (currently only the
  provider index page).

After changing any of the above, regenerate:

```shell
make generate
```

Commit the updated files in `docs/` alongside your code changes. Do not edit
files in `docs/` by hand.

---

## Commit messages and pull requests

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
The branch ruleset enforces squash-only merges, so the **PR title becomes the
commit message on `main`** for every PR, regardless of how many commits it
contains. CI validates the PR title and blocks merge if it is not a valid
conventional commit.

The title must follow the format:

```
<type>(<optional scope>): <subject starting with a lowercase letter>
```

Accepted types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`,
`build`, `ci`, `chore`, `revert`.

Individual commits within a PR branch do not need to follow this format — only
the PR title matters, since that is what lands on `main`.

---

## Releasing

Releases are fully automated via [release-please](https://github.com/googleapis/release-please)
and [GoReleaser](https://goreleaser.com/). **No manual tagging required.**

### How it works

1. Every merge to `main` triggers the `cd.yml` workflow.
2. **release-please** scans conventional commits since the last release and
   opens (or updates) a **Release PR** containing the computed next version,
   an updated `CHANGELOG.md`, and a bump to `.release-please-manifest.json`.
3. When a maintainer **merges the Release PR**, release-please creates the git
   tag (e.g. `v0.3.0`).
4. The same workflow detects the new tag and runs **GoReleaser**, which:
   - Builds binaries for all target platforms
   - Signs checksums with the repository GPG key
   - Creates the GitHub Release and uploads all assets
5. The Terraform Registry picks up the release automatically via its GitHub
   webhook within a few minutes.

### Commit type → version bump mapping

| Commit prefix | Example | SemVer bump |
|---|---|---|
| `fix:` | `fix: handle nil pointer in auth` | patch (0.2.0 → 0.2.1) |
| `feat:` | `feat: add robot accounts resource` | minor (0.2.0 → 0.3.0) |
| `feat!:` / `BREAKING CHANGE:` | `feat!: remove deprecated endpoint` | major (0.2.0 → 1.0.0) |
| `chore:`, `ci:`, `docs:`, `test:`, `refactor:` | — | no release |

### Updating the provider version constraint in docs

When a new minor version is released (e.g. `v0.3.0`), update the version
constraint in `examples/provider/provider.tf` from `~> 0.2` to `~> 0.3` as
part of a follow-up PR. The CI docs-drift check will catch any mismatch between
the example files and the generated `docs/` output.

### First-time release setup

The following one-time steps are required before the automated release pipeline
can run successfully from a new repository fork or clone.

#### 1. Generate a GPG signing key

The Terraform Registry requires RSA or DSA keys — ECC (ed25519) keys are
rejected.

```shell
gpg --full-generate-key
# Choose: (1) RSA and RSA, 4096 bits
```

#### 2. Upload the public key to the Terraform Registry

```shell
gpg --armor --export your-email@example.com
```

Paste the output at <https://registry.terraform.io/settings/gpg-keys>.

#### 3. Add secrets to the GitHub repository

Navigate to **Settings → Secrets and variables → Actions** and add:

| Secret | Value |
|---|---|
| `GPG_PRIVATE_KEY` | `gpg --armor --export-secret-keys YOUR_KEY_ID` |
| `PASSPHRASE` | The passphrase chosen when generating the key |

#### 4. Register the provider on the Terraform Registry

1. Sign in to [registry.terraform.io](https://registry.terraform.io) with GitHub.
2. Click **Publish → Provider**.
3. Select the `terraform-provider-quay` repository.
4. Accept the Terms of Use and click **Publish Provider**.

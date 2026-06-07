---
name: kubearchive-contributing
description: Guide for contributing to KubeArchive — PR workflow, CI pipeline, testing commands, code review process, and release cycle. Use this skill when someone wants to submit a PR, run tests, understand CI failures, prepare code for review, check linting, understand the release process, or asks about "contributing", "PR template", "make test", "CI pipeline", "release notes", "reviewer assignment", or "how to submit changes" in KubeArchive.
---

# Contributing to KubeArchive

## Quick Reference

```bash
make generate       # Regenerate CRDs, RBAC, DeepCopy (run after changing CRD types)
make build          # Build all binaries (runs generate first)
make fmt            # Format code with goimports
make lint           # Run golangci-lint
make test           # Unit tests with envtest
make test-short     # Unit tests, skip slow ones
make test-race      # Unit tests with race detector + coverage
make test-integration  # Integration tests (requires Kind cluster with KubeArchive)
make check          # lint + unit tests combined
```

## PR Workflow

### 1. Branch and Develop

Fork the repo, create a feature branch, make changes. The main branch is `main`.

### 2. Run Checks Locally

Before pushing, run at minimum:

```bash
make check    # lint + unit tests
```

For operator or CRD changes, also run `make generate` first — the CI will fail if generated code is stale.

### 3. PR Template Requirements

Every PR must include:

**Issue link** (required):
```markdown
Resolves #123     <!-- auto-closes issue on merge -->
Related to #123   <!-- links without auto-close -->
```

**Release note** (required) — use a fenced code block:
````markdown
```release-note
Added support for wildcard name filtering in API queries
```
````

Or for non-user-facing changes:
````markdown
```release-note
NONE
```
````

**Labels** (required) — at least one of:
- `kind/bug`, `kind/feature`, `kind/documentation`
- Add `kind/breaking` for breaking changes

**Reviewer notes** (optional) — highlight specific areas you want reviewers to focus on.

### 4. CI Pipeline

These checks run automatically on every PR:

| Check | What it does | Config |
|-------|-------------|--------|
| **Unit Tests** | `go test` with envtest (Go 1.25.5) | `.github/workflows/go_test.yml` |
| **Integration Tests** | Kind cluster + full KubeArchive install | `.github/workflows/go_test_integration.yml` |
| **Linting** | golangci-lint (new issues only on PRs) | `.github/workflows/golangci_lint.yml` |
| **Static Analysis** | `go mod tidy` check, yamllint, license check | `.github/workflows/static_code_analysis.yml` |
| **Migration Tests** | Database schema migrations against PostgreSQL | `.github/workflows/migration_test.yml` |
| **Secret Scanning** | gitleaks for accidental credential commits | `.github/workflows/gitleaks.yml` |

All checks must pass before merge.

### 5. Reviewer Assignment

Reviewers are auto-assigned based on PR labels via `.github/assign_label_reviewers.yml`:

| Label | Reviewers |
|-------|-----------|
| `area/ci` | maruiz93, rh-hemartin |
| `area/database` | mafh314 |
| `area/deployment` | rh-hemartin, skoved |
| `component/api` | maruiz93 |
| `component/cli` | maruiz93, rh-hemartin |
| `component/operator` | ggallen |
| `component/sink` | skoved |

Add the appropriate `area/*` or `component/*` label to get the right reviewer.

## Testing

### Unit Tests

Unit tests live alongside the code they test (`*_test.go` files). They use Go's standard `testing` package plus controller-runtime's `envtest` for operator tests.

```bash
make test            # all unit tests
make test-short      # skip slow tests (useful during development)
make test-race       # with race detector and coverage report
```

The envtest framework starts a local Kubernetes API server (etcd + kube-apiserver) so controller tests can interact with real Kubernetes APIs without a full cluster.

### Integration Tests

Integration tests live in `test/integration/` and require a running Kind cluster with KubeArchive installed.

```bash
# Setup (if not already running)
kind create cluster
export KO_DOCKER_REPO="kind.local"
hack/quick-install.sh

# Run integration tests
make test-integration
```

Integration tests cover end-to-end flows: operator reconciliation, API queries, database operations, pagination, label filtering, impersonation, and vacuum operations.

The integration test runner (`test/integration/run.sh`) outputs JSON-lines format and saves timestamped results to `integration-results/`.

### What to Test

When changing code, run the relevant tests:

| Changed | Run |
|---------|-----|
| CRD types or webhooks | `make generate && make test` |
| API server routes or middleware | `make test` + integration tests |
| Database queries or migrations | `make test` + migration tests |
| Operator controllers | `make test` (envtest covers controllers) |
| Sink CloudEvent handling | `make test` |
| Anything | `make check` at minimum |

## Linting

KubeArchive uses golangci-lint with these enabled linters (configured in `.golangci.yml`):

- **bodyclose** — checks HTTP response body is closed
- **gosec** — security-focused checks
- **nilerr** — catches returning nil when err is non-nil
- **shadow** — detects variable shadowing
- **unparam** — finds unused function parameters
- **wastedassign** — detects assignments that are never read

The CI runs golangci-lint in "new issues only" mode on PRs — it only flags problems in your changed code, not pre-existing issues.

```bash
make lint    # run locally to catch issues before pushing
make fmt     # auto-format with goimports
```

## Code Generation

KubeArchive uses `controller-gen` (via `make generate`) to produce:

- CRD YAML manifests from Go type annotations (`//+kubebuilder:...`)
- RBAC role manifests from controller annotations (`//+kubebuilder:rbac:...`)
- DeepCopy methods for CRD types

Always run `make generate` after modifying:
- CRD type definitions in `cmd/operator/api/v1/`
- `//+kubebuilder:rbac` annotations in controller files
- Webhook marker annotations

The CI checks that generated code is up to date — if you forget `make generate`, the static analysis check will fail.

## File Headers

All Go source files must have an Apache 2.0 SPDX license header. Check existing files for the exact format.

## Release Process

Releases happen bi-monthly (1st and 15th) via the automated `release.yml` workflow. The process:

1. `hack/release.sh` runs the Kubernetes release notes generator
2. Release notes are aggregated from PR `release-note` blocks
3. Container images are built with `ko` and published to the OCI registry
4. A GitHub release is created with the generated changelog

The current version lives in the `VERSION` file at the repo root.

## Project Structure for Contributors

When adding new functionality, follow these patterns:

- **New API endpoint:** Add routes in `cmd/api/routers/routers.go`, add database query methods if needed
- **New CRD field:** Update types in `cmd/operator/api/v1/`, add webhook validation, run `make generate`
- **New database query:** Add to `pkg/database/interfaces/database.go` interface, implement in `pkg/database/sql/`
- **New integration:** Add under `integrations/` with its own README
- **New CLI command:** Add to `pkg/cmd/` and register in `cmd/kubectl-ka/main.go`

## Governance

- **Maintainers:** Listed in `MAINTAINERS.md` (6 Red Hat maintainers)
- **Code of Conduct:** Contributor Covenant (`CODE_OF_CONDUCT.md`)
- **License:** Apache 2.0
- **Full contributor guide:** https://kubearchive.github.io/kubearchive/main/contributors/guide.html

## Project Overview

KubeArchive archives Kubernetes resources to a JSON-capable relational database (PostgreSQL, MariaDB, or MySQL), allowing cluster cleanup without data loss.

## Repository Structure

- `cmd/` — Binaries: `api`, `sink`, `operator`, `vacuum`, `installer`, `kubectl-ka`
- `pkg/` — Shared libraries (database, CLI, filters, CEL, observability)
- `config/` — Kustomize manifests and CRDs
- `hack/` — Dev/install/release scripts
- `test/` — Integration and performance tests
- `integrations/` — Database, logging, observability setup

## Prerequisites

Code generation MUST run before building or testing:

```bash
bash cmd/operator/generate.sh
bash cmd/installer/generate.sh
```

## Build

```bash
go build ./...
```

Container images are built with `ko` (`.ko.yaml`). There is no Makefile.

## Test

Unit tests require envtest binaries for operator controllers:

```bash
export KUBEBUILDER_ASSETS=$(cmd/operator/bin/setup-envtest use --bin-dir cmd/operator/bin -p path)
go test ./...
```

Integration tests require a KinD cluster with KubeArchive installed (see `hack/quick-install.sh`):

```bash
go test -tags=integration ./test/integration/...
```

## Lint

```bash
golangci-lint run
```

## Key Conventions
- Operator uses kubebuilder/controller-runtime patterns
- CRDs belong to API group `kubearchive.org`
- Deployment: `bash hack/kubearchive-install.sh`
- Do not edit files in `config/crds/`; re-run code generation

## Security Conventions

- **Never log credential values in test code.** Do not pass Secret data (passwords, tokens, connection strings — including base64-encoded forms) to `t.Log`, `t.Logf`, `fmt.Print`, or `log.Print`. Use a placeholder like `[REDACTED]` in log messages.
- **Suppress shell tracing around credentials.** In shell scripts that handle `DB_PASSWORD`, `DATABASE_URL`, or other Secret values, disable `set -x` / `set -o xtrace` before reading or writing credential variables, and re-enable after.
- **Use context with timeout for pod exec.** Integration tests that execute commands inside pods (e.g., `psql` via `StreamWithContext`) must use `context.WithTimeout` to prevent indefinite hangs if the command stalls.
- **Kubernetes Secrets ship with empty values.** The database credential Secret template (`config/templates/database/database_secret.yaml`) uses empty `stringData` fields. Install scripts (`hack/kubearchive-install.sh`, `integrations/database/postgresql/install.sh`) populate credentials at runtime. Reviews should verify that install scripts preserve existing credentials across `ko apply`.

## Review
Last reviewed: Q2 2026. Next review: Q3 2026. See [workflow](.github/workflows/agents-md-review.yml).

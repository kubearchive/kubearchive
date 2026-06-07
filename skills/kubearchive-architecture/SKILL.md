---
name: kubearchive-architecture
description: Understand KubeArchive's three-component architecture (Operator, Sink, API Server), data flow, database schema, and codebase layout. Use this skill whenever someone asks how KubeArchive works, what a component does, where code lives, how data flows from Kubernetes to the archive, or needs to understand the system before making changes. Also use when you see questions about "kubearchive components", "archive flow", "sink", "operator", "api-server", or "how does kubearchive store resources".
---

# KubeArchive Architecture

KubeArchive is a read-archival system for ephemeral Kubernetes resources. It captures resources (Pods, Jobs, PipelineRuns, etc.) before they disappear from the cluster and stores them in a PostgreSQL database for later retrieval through a Kubernetes-compatible REST API.

## Components

KubeArchive has three main components plus supporting utilities:

```
Kubernetes API Server
        |
        | (watches resources)
        v
  +-----------+     CloudEvents (HTTP)     +------+     SQL INSERT     +------------+
  |  Operator  | ========================> |  Sink | ================> | PostgreSQL |
  +-----------+                            +------+                   +------------+
                                                                            |
                                                                       SQL SELECT
                                                                            |
                                                                      +------------+
                                                                      | API Server |
                                                                      +------------+
                                                                            ^
                                                                            |
                                                                     HTTPS REST API
                                                                            |
                                                                   kubectl-ka / curl
```

### Operator (`cmd/operator/`)

The operator is a controller-runtime application that decides *what* to archive. It:

- Watches `KubeArchiveConfig` and `ClusterKubeArchiveConfig` CRDs to learn which resource types to archive
- Aggregates archival rules into a `SinkFilter` resource
- Dynamically creates Kubernetes watches for each resource type in the SinkFilter
- Evaluates CEL (Common Expression Language) expressions on watch events to decide whether to archive
- Wraps matching resources in CloudEvents and sends them to the Sink via HTTP POST
- Manages RBAC dynamically — creates Roles, RoleBindings, and ServiceAccounts so the Sink can delete archived resources

Key reconcilers:
- `KubeArchiveConfigReconciler` — handles namespace-scoped archival configs
- `ClusterKubeArchiveConfigReconciler` — handles cluster-wide archival configs
- `SinkFilterReconciler` — manages the actual resource watches and CEL evaluation

### Sink (`cmd/sink/`)

The sink is a Gin HTTP server that receives CloudEvents and writes to the database. It:

- Listens on port 8080 (HTTP, internal-only)
- Extracts Kubernetes resource manifests from CloudEvent payloads
- Stores the full resource JSON in PostgreSQL (JSONB column)
- Builds and stores log URLs for Pod containers (supports Elasticsearch, Splunk, Datadog)
- Normalizes and stores resource labels for efficient querying

The sink is the only component that writes to the database.

### API Server (`cmd/api/`)

The API server is a Gin HTTPS server that provides read access to archived resources. It:

- Listens on port 8081 (TLS, cert-manager certificates)
- Serves a Kubernetes-compatible REST API (`/api/` and `/apis/` paths)
- Authenticates requests using Kubernetes TokenReview (validates bearer tokens)
- Authorizes requests using SubjectAccessReview (respects cluster RBAC)
- Supports pagination, label selectors, timestamp filtering, and wildcard name matching
- Streams large result sets to avoid memory pressure

The API server only reads from the database.

### Supporting Components

- **kubectl-ka** (`cmd/kubectl-ka/`) — kubectl plugin for querying archived resources from the CLI
- **Vacuum** (`cmd/vacuum/`) — enforces retention policies, deleting old archived resources
- **Installer** (`cmd/installer/`) — handles installation CRD lifecycle

## Custom Resource Definitions

All CRDs are in `cmd/operator/api/v1/`:

| CRD | Scope | Short Names | Purpose |
|-----|-------|-------------|---------|
| `KubeArchiveConfig` | Namespace | `kac`, `kacs` | Define which resources to archive in a namespace |
| `ClusterKubeArchiveConfig` | Cluster | `ckac`, `ckacs` | Define cluster-wide archival policies |
| `SinkFilter` | Namespace | `sf`, `sfs` | Aggregated view of all archival rules (managed by operator) |
| `NamespaceVacuumConfig` | Namespace | — | Retention policies per namespace |
| `ClusterVacuumConfig` | Cluster | — | Cluster-wide retention policies |

### KubeArchiveConfig Fields

```yaml
apiVersion: kubearchive.org/v1
kind: KubeArchiveConfig
metadata:
  name: kubearchive  # must be named "kubearchive"
  namespace: my-namespace
spec:
  resources:
    - selector:
        apiVersion: batch/v1
        kind: Job
      archiveWhen: "status.active == 0"        # CEL: when to archive
      deleteWhen: "has(status.completionTime)"  # CEL: when to delete from cluster
      archiveOnDelete: "true"                   # CEL: archive on resource deletion
```

CEL expressions receive the full Kubernetes resource as context and must return a boolean.

## Data Flow

### Write Path (Archival)

1. User creates a `KubeArchiveConfig` in their namespace
2. Operator reconciles: updates `SinkFilter`, creates RBAC, starts watches
3. `SinkFilterReconciler` creates dynamic watches for specified resource types
4. When a watched resource is created/updated/deleted, the operator evaluates CEL expressions
5. If a CEL condition matches, the resource is wrapped in a CloudEvent and POSTed to the Sink
6. Sink extracts the resource, stores it in PostgreSQL (JSONB), normalizes labels, and builds log URLs

### Read Path (Retrieval)

1. Client sends HTTPS request to API server (e.g., `GET /apis/batch/v1/namespaces/default/jobs`)
2. API server authenticates via Kubernetes TokenReview
3. API server authorizes via SubjectAccessReview (checks if user can `list` the resource type)
4. Database query: filtered by kind, apiVersion, namespace, labels, timestamps
5. Results streamed back as a Kubernetes-style JSON list with pagination support

## Database Schema

PostgreSQL is the primary database. Schema lives in `integrations/database/postgresql/migrations/`.

### Core Tables

```
resource              — Full resource manifests (JSONB), keyed by UUID
log_url               — Log URLs per container, linked to resource by UUID
label_key             — Normalized label keys
label_value           — Normalized label values
label_key_value       — Key-value pairs (deduplicated)
resource_label        — Many-to-many: resource ↔ label
```

The `resource` table stores the complete Kubernetes resource JSON in a `data` column (JSONB). This is what gets returned by the API server. Labels are normalized into separate tables for efficient label selector queries.

## Codebase Layout

```
cmd/
  api/              — API server binary and route handlers
  operator/         — Operator binary, controllers, CRD types, webhooks
  sink/             — Sink binary and CloudEvent handler
  kubectl-ka/       — kubectl plugin
  vacuum/           — Retention enforcement
  installer/        — Installation CRD handler

pkg/
  abort/            — HTTP error helper (Gin middleware)
  cache/            — In-memory cache with TTL (used for auth caching)
  cel/              — CEL expression compilation and evaluation
  cloudevents/      — CloudEvent publisher for Sink communication
  cmd/              — kubectl-ka command implementations (get, logs, config)
  database/         — Database abstraction (DBReader, DBWriter interfaces)
    interfaces/     — Database interface definitions
    sql/            — SQL query building and execution
    errors/         — Custom database errors (ErrResourceNotFound)
    env/            — Database connection environment variables
  discovery/        — Kubernetes API discovery helpers
  logging/          — Structured logging setup (slog + context handler)
  models/           — Data models (Resource, LogURL)
  observability/    — OpenTelemetry tracing and metrics setup

config/             — Kustomize manifests for deployment
integrations/
  database/         — Database migrations and setup
  logging/          — Log provider configurations (Elasticsearch, Splunk, Datadog)
  observability/    — Grafana dashboards, Prometheus ServiceMonitors

hack/               — Build and install scripts
test/               — Integration tests, debug tools, performance tests
```

## Build and Deploy

KubeArchive uses **ko** for building container images and **Kustomize** for deployment manifests. There is no Helm chart.

```bash
export KO_DOCKER_REPO="kind.local"   # for local dev
hack/quick-install.sh                 # full install (PostgreSQL + cert-manager + KubeArchive)
hack/kubearchive-install.sh           # redeploy KubeArchive only (after code changes)
hack/kubearchive-delete.sh            # uninstall
```

## Key Design Decisions

- **CloudEvents for decoupling:** The operator and sink communicate via CloudEvents over HTTP, keeping them loosely coupled and independently deployable.
- **Kubernetes-compatible API:** The API server mimics Kubernetes API patterns (`/apis/{group}/{version}/...`) so existing tools and mental models transfer directly.
- **RBAC passthrough:** The API server delegates authentication and authorization to the Kubernetes API server itself, so archived resources respect the same access controls as live resources.
- **CEL for filtering:** CEL expressions give users fine-grained control over what gets archived without requiring code changes.
- **JSONB storage:** Storing full resource manifests as JSONB preserves all fields and allows PostgreSQL's JSON operators for future query capabilities.
- **Normalized labels:** Labels are normalized into separate tables because label selector queries are the most common filter pattern and need to be fast.

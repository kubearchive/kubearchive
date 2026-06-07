---
name: kubearchive-api
description: Query archived Kubernetes resources through the KubeArchive API server and kubectl-ka CLI plugin. Covers route patterns, authentication, query parameters, pagination, label selectors, log retrieval, and response formats. Use this skill when someone wants to query archived resources, set up API access, use kubectl-ka, understand response formats, filter by labels or timestamps, paginate results, retrieve archived pod logs, or build integrations against the KubeArchive API. Also trigger on "kubectl ka", "query kubearchive", "archived resources", "kubearchive API", "label selector archive", or "get old pods/jobs/pipelines".
---

# KubeArchive API

The KubeArchive API server provides a Kubernetes-compatible REST API for querying archived resources. It uses the same authentication and authorization as the Kubernetes API server — if you can `list` a resource type in a namespace on the live cluster, you can query its archived versions too.

## Authentication

Every request requires a bearer token in the `Authorization` header. The API server validates tokens using Kubernetes TokenReview.

```bash
# Create a token for a service account
TOKEN=$(kubectl create -n <namespace> token <service-account>)

# Use it
curl -s --cacert ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  https://localhost:8081/apis/batch/v1/namespaces/<namespace>/jobs
```

For local development, set up test users first:

```bash
kubectl apply -f test/users/    # creates test namespace + service accounts with RBAC
```

### TLS Certificate

The API server uses TLS. Extract the CA certificate for curl:

```bash
kubectl get -n kubearchive secrets kubearchive-api-server-tls \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
```

On OpenShift with a local port-forward, you can skip the CA cert with `-k` and use your user token. **Only use `-k` against localhost port-forwards, never against remote endpoints** — it disables server identity verification and exposes your token to MITM attacks:

```bash
curl -k -H "Authorization: Bearer $(oc whoami --show-token)" \
  https://localhost:8081/apis/batch/v1/jobs
```

### Impersonation

When `AUTH_IMPERSONATE=true` is set on the API server (local development only — do not enable in production without understanding the RBAC implications), you can impersonate other users:

```bash
curl -s --cacert ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Impersonate-User: jane@example.com" \
  -H "Impersonate-Group: developers" \
  https://localhost:8081/apis/batch/v1/namespaces/default/jobs
```

The impersonating user must have impersonation permissions in RBAC.

## Route Patterns

The API mirrors Kubernetes API paths:

### Core API Resources (`/api/`)

```
GET /api/{version}/{resourceType}
GET /api/{version}/namespaces/{namespace}/{resourceType}
GET /api/{version}/namespaces/{namespace}/{resourceType}/{name}
GET /api/{version}/namespaces/{namespace}/{resourceType}/{name}/log
GET /api/{version}/namespaces/{namespace}/{resourceType}/uid/{uid}
GET /api/{version}/namespaces/{namespace}/{resourceType}/uid/{uid}/log
```

Example: `GET /api/v1/namespaces/default/pods`

### Grouped API Resources (`/apis/`)

```
GET /apis/{group}/{version}/{resourceType}
GET /apis/{group}/{version}/namespaces/{namespace}/{resourceType}
GET /apis/{group}/{version}/namespaces/{namespace}/{resourceType}/{name}
GET /apis/{group}/{version}/namespaces/{namespace}/{resourceType}/{name}/log
GET /apis/{group}/{version}/namespaces/{namespace}/{resourceType}/uid/{uid}
GET /apis/{group}/{version}/namespaces/{namespace}/{resourceType}/uid/{uid}/log
```

Example: `GET /apis/tekton.dev/v1/namespaces/build/pipelineruns`

### Cluster-Wide Queries

Omit the `namespaces/{namespace}` segment to query across all namespaces (requires cluster-wide RBAC):

```
GET /apis/batch/v1/jobs              # all jobs in all namespaces
GET /api/v1/pods                     # all pods in all namespaces
```

## Query Parameters

| Parameter | Format | Description |
|-----------|--------|-------------|
| `limit` | Integer (1-1000) | Max results per page. Default: 100 |
| `continue` | Base64 string | Pagination token from previous response |
| `labelSelector` | Kubernetes selector | Filter by labels (see below) |
| `name` | String (supports `*` wildcard) | Filter by resource name |
| `creationTimestampAfter` | RFC3339 | Only resources created after this time |
| `creationTimestampBefore` | RFC3339 | Only resources created before this time |

### Label Selectors

Standard Kubernetes label selector syntax:

```bash
# Exact match
?labelSelector=app=myapp

# Multiple conditions (AND)
?labelSelector=app=myapp,env=prod

# Not equal
?labelSelector=env!=staging

# Set-based
?labelSelector=env%20in%20(prod,staging)
?labelSelector=env%20notin%20(test)

# Existence
?labelSelector=app
?labelSelector=!deprecated
```

### Wildcard Name Filtering

Use `*` in the name query parameter for pattern matching:

```bash
# All jobs starting with "build-"
?name=build-*

# All pods containing "worker"
?name=*worker*
```

### Timestamp Filtering

```bash
# Resources created in the last 24 hours
?creationTimestampAfter=2024-01-15T00:00:00Z

# Resources created in a specific window
?creationTimestampAfter=2024-01-01T00:00:00Z&creationTimestampBefore=2024-01-31T23:59:59Z
```

## Pagination

Results are paginated. The default page size is 100 (max 1000).

```bash
# First page
curl -s --cacert ca.crt -H "Authorization: Bearer $TOKEN" \
  "https://localhost:8081/apis/batch/v1/namespaces/default/jobs?limit=10" | jq

# The response includes a continue token if there are more results:
# { "metadata": { "continue": "eyJpZCI6..." } }

# Next page
curl -s --cacert ca.crt -H "Authorization: Bearer $TOKEN" \
  "https://localhost:8081/apis/batch/v1/namespaces/default/jobs?limit=10&continue=eyJpZCI6..."
```

When `metadata.continue` is absent or empty, you've reached the last page.

## Response Format

### List Response

```json
{
  "kind": "List",
  "apiVersion": "v1",
  "items": [
    {
      "apiVersion": "batch/v1",
      "kind": "Job",
      "metadata": { "name": "my-job", "namespace": "default", ... },
      "spec": { ... },
      "status": { ... }
    }
  ],
  "metadata": {
    "continue": "eyJpZCI6MTIzLCJkYXRlIjoiMjAyNC0wMS0xNVQxMjowMDowMFoifQ=="
  }
}
```

Items contain the full Kubernetes resource manifest exactly as it was when archived.

### Single Resource Response

When querying by exact name or UID, the response is the raw resource (not wrapped in a list):

```json
{
  "apiVersion": "batch/v1",
  "kind": "Job",
  "metadata": { "name": "my-job", ... },
  "spec": { ... },
  "status": { ... }
}
```

### Error Responses

```json
{
  "code": 401,
  "message": "token review: unauthorized"
}
```

| Code | Meaning |
|------|---------|
| 400 | Bad request (invalid query parameters) |
| 401 | Unauthorized (invalid or missing token) |
| 404 | Resource not found |
| 500 | Internal server error |
| 503 | Service unavailable (database down) |

## Log Retrieval

Archived pod logs can be retrieved if a logging integration is configured (Elasticsearch, Splunk, or Datadog):

```bash
# By pod name
GET /api/v1/namespaces/{ns}/pods/{name}/log

# By pod UID
GET /api/v1/namespaces/{ns}/pods/uid/{uid}/log
```

The response is plain text with these headers:

```
Content-Type: text/plain; charset=utf-8
X-Pod-Name: my-pod
X-Pod-UUID: abc-123-def
X-Container-Name: main
```

Log retrieval requires both `get pods` and `get pods/log` permissions in RBAC.

## kubectl-ka Plugin

The `kubectl-ka` plugin provides CLI access to the KubeArchive API.

### Configuration

```bash
# Set the API server URL and certificate
kubectl ka config set host https://localhost:8081
kubectl ka config set cert-path ./ca.crt

# Or use environment variables
export KUBECTL_PLUGIN_KA_HOST=https://localhost:8081
export KUBECTL_PLUGIN_KA_CERT_PATH=./ca.crt
export KUBECTL_PLUGIN_KA_TLS_INSECURE=true   # local dev only — unset after use
export KUBECTL_PLUGIN_KA_TOKEN=<bearer-token>  # override token
```

### Querying Resources

```bash
# List archived jobs in a namespace
kubectl ka get jobs -n default

# Get a specific resource
kubectl ka get jobs my-job -n default

# All namespaces
kubectl ka get jobs -A

# With label selector
kubectl ka get pods -n default -l app=myapp

# With timestamp filter
kubectl ka get pipelineruns -n build --after 2024-01-15T00:00:00Z

# JSON or YAML output
kubectl ka get jobs -n default -o json
kubectl ka get jobs -n default -o yaml

# Limit results
kubectl ka get pods -n default --limit 50
```

### Resource Format

Resources can be specified as:
- `pods`, `jobs`, `pipelineruns` — just the type
- `pods.v1` — with version
- `pipelineruns.tekton.dev/v1` — with group and version

### Live + Archived Results

By default, `kubectl ka get` returns both live cluster resources and archived resources. Control this with flags:

```bash
kubectl ka get pods -n default                    # both live and archived
kubectl ka get pods -n default --archived=false   # live only
kubectl ka get pods -n default --in-cluster=false # archived only
```

### Log Retrieval

```bash
kubectl ka logs my-pod -n default
kubectl ka logs my-pod -n default --container main
```

## Health Endpoints

```
GET /livez    # liveness: returns server config (always 200 if server is up)
GET /readyz   # readiness: pings database (503 if DB is unreachable)
```

These don't require authentication.

## Building Integrations

When building applications that query the KubeArchive API:

1. **Use the same API paths as Kubernetes** — if you know how to query the Kubernetes API, use the same paths against KubeArchive
2. **Reuse Kubernetes client libraries** — the response format is standard Kubernetes JSON, so `client-go`'s `unstructured.Unstructured` works directly
3. **Handle pagination** — always follow `metadata.continue` tokens for complete results
4. **Cache tokens** — `kubectl create token` creates short-lived tokens; cache and refresh as needed
5. **Use label selectors** — they're the most efficient filter because labels are indexed in the database

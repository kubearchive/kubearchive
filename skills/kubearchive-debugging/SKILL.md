---
name: kubearchive-debugging
description: Diagnose and fix issues in KubeArchive components — API server, sink, and operator. Covers structured logging, health endpoints, pprof profiling, OpenTelemetry tracing, database troubleshooting, and common error patterns. Use this skill when something is broken in KubeArchive, pods are crashing, the API returns errors, resources aren't being archived, performance is degraded, or the user wants to set up observability. Also trigger on "kubearchive logs", "debug kubearchive", "kubearchive not working", "archived resources missing", "API 401/403/500", or "kubearchive observability".
---

# Debugging KubeArchive

## First Steps

When something isn't working, start with pod status and logs for the relevant component:

```bash
# Check all KubeArchive pods
kubectl get pods -n kubearchive

# Component logs
kubectl logs -n kubearchive -l app=kubearchive-api-server
kubectl logs -n kubearchive -l app=kubearchive-sink
kubectl logs -n kubearchive -l app=kubearchive-operator
```

Then check health endpoints:

```bash
# API server health (requires port-forward)
kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081
curl -s --cacert ca.crt https://localhost:8081/livez | jq    # server config and health
curl -s --cacert ca.crt https://localhost:8081/readyz | jq   # database connectivity
```

If you don't have `ca.crt` extracted yet:
```bash
kubectl get -n kubearchive secrets kubearchive-api-server-tls \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
```

The `/readyz` endpoint returns 503 if the database connection is down — this is the fastest way to check if the API server can reach PostgreSQL.

## Logging

KubeArchive uses Go's `log/slog` for structured logging. Every log line includes trace-id and span-id from the request context (when OpenTelemetry is enabled), which lets you correlate logs across components for a single request.

### Log Levels

| Variable | Controls | Default | Values |
|----------|----------|---------|--------|
| `LOG_LEVEL` | slog output level | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `KLOG_LEVEL` | Kubernetes client logging | `0` | `0`-`10` (higher = more verbose) |

To enable debug logging, edit the deployment:

```bash
kubectl set env -n kubearchive deployment/kubearchive-api-server LOG_LEVEL=DEBUG
```

### What to Look For in Each Component

**API Server logs:**
- `"there was a problem"` — request-level errors with HTTP status code
- Database connection failures on startup
- Auth cache TTL configuration (logged at startup)
- Request latency (from Logger middleware)

**Sink logs:**
- CloudEvent parsing errors (HTTP 400 response to operator)
- Malformed event payloads (HTTP 422)
- Database write failures
- Log URL building errors for Pod containers

**Operator logs:**
- Watch creation/deletion for resource types
- CEL expression evaluation results
- RBAC reconciliation (Role/RoleBinding creation)
- CloudEvent publishing failures (can't reach sink)
- Backoff/retry on watch reconnection

## Common Problems

### Resources Not Being Archived

**Symptoms:** API returns empty lists, database has no new entries.

**Check in order:**

1. **KubeArchiveConfig exists and is valid:**
   ```bash
   kubectl get kac -A                    # list all configs
   kubectl describe kac kubearchive -n <namespace>  # check for errors
   ```

2. **SinkFilter reflects the config:**
   ```bash
   kubectl get sinkfilter -n kubearchive -o yaml
   ```
   The SinkFilter should list the resource types from your KubeArchiveConfig. If it's empty, the operator isn't reconciling — check operator logs.

3. **Operator is watching the right resources:**
   Look for watch creation logs in the operator. If watches are being created but nothing is being archived, the CEL expression may not be matching.

4. **CEL expression is correct:**
   CEL expressions receive the full Kubernetes resource. Common mistakes:
   - Using `status.active == 0` on a resource that doesn't have `status.active`
   - Missing `has()` checks for optional fields: use `has(status.completionTime)` instead of `status.completionTime != ""`

5. **Sink is reachable:**
   ```bash
   kubectl get svc -n kubearchive kubearchive-sink  # should exist
   kubectl logs -n kubearchive -l app=kubearchive-sink  # check for incoming events
   ```

### API Returns 401 Unauthorized

The API server validates bearer tokens using Kubernetes TokenReview. Common causes:

- **Expired token:** Service account tokens from `kubectl create token` expire. Create a fresh one.
- **Wrong namespace:** The token must come from a service account in the namespace you're querying (or one with cluster-wide access).
- **Missing test users:** For local dev, apply test users: `kubectl apply -f test/users/`

### API Returns 403 Forbidden

The API server checks RBAC via SubjectAccessReview. The authenticated user must have `get` or `list` permissions on the resource type being queried in the target namespace.

```bash
# Check if a service account can list jobs in a namespace
kubectl auth can-i list jobs -n <namespace> --as=system:serviceaccount:<namespace>:<sa-name>
```

### API Returns Empty Results (But Resources Exist in DB)

- **Wrong API path:** Core resources use `/api/v1/...`, grouped resources use `/apis/{group}/{version}/...`
- **Wrong namespace:** Check you're querying the right namespace
- **Pagination:** Default limit is 100. Use `?limit=1000` or follow `metadata.continue` tokens

### Pods CrashLooping

```bash
kubectl describe pod -n kubearchive <pod-name>  # check Events section
kubectl logs -n kubearchive <pod-name> --previous  # logs from crashed container
```

Common causes:
- **OOMKilled:** Increase memory limits in the deployment
- **Database unreachable:** Check the PostgreSQL pod and credentials secret
- **TLS certificate errors:** cert-manager may not have issued certificates yet

### Database Connection Issues

The database connection is configured via environment variables. Check them:

```bash
kubectl get secret -n kubearchive kubearchive-database-credentials -o yaml
```

Required env vars (injected from the secret):
- `DATABASE_KIND` — `postgres` (or `mysql`, `mariadb`)
- `DATABASE_URL` — hostname
- `DATABASE_PORT` — port number
- `DATABASE_DB` — database name
- `DATABASE_USER` — username
- `DATABASE_PASSWORD` — password

The connection retries 10 times with 1-second delays on startup. If all retries fail, the pod will crash.

To connect directly to the database:

```bash
# Find the PostgreSQL pod
kubectl get pods -n kubearchive -l role=primary

# Exec into it
kubectl exec -it -n kubearchive <pg-pod> -- psql -U <user> -d <db>

# Useful queries
SELECT count(*) FROM resource;
SELECT kind, api_version, count(*) FROM resource GROUP BY kind, api_version;
SELECT * FROM resource ORDER BY created_at DESC LIMIT 5;
```

## Profiling with pprof

The API server and sink can expose pprof endpoints for CPU and memory profiling. Enable it:

```bash
kubectl set env -n kubearchive deployment/kubearchive-api-server KUBEARCHIVE_ENABLE_PPROF=true
```

pprof runs on a separate TLS server at port 8888. **Always access it via port-forward only** — pprof endpoints expose sensitive runtime data (goroutine stacks, heap contents) and must never be exposed outside the pod:

```bash
kubectl port-forward -n kubearchive pod/<api-pod> 8888:8888
go tool pprof https+insecure://localhost:8888/debug/pprof/profile?seconds=30  # CPU
go tool pprof https+insecure://localhost:8888/debug/pprof/heap               # memory
```

## OpenTelemetry Tracing and Metrics

KubeArchive integrates with OpenTelemetry for distributed tracing and metrics.

### Configuration

| Variable | Purpose | Values |
|----------|---------|--------|
| `KUBEARCHIVE_OTEL_MODE` | Tracing mode | `disabled`, `enabled`, `delegated` |
| `OTEL_TRACES_SAMPLER_ARG` | Sampling rate | `0.0` to `1.0` (default: `1.0` = all traces) |
| `KUBEARCHIVE_METRICS_INTERVAL` | Metrics flush interval | Duration (default: `1m`) |
| `KUBEARCHIVE_OTLP_SEND_LOGS` | Export logs to OTLP | `true` / `false` |

### Setting Up the Observability Stack

Install the Grafana LGTM stack (Tempo for traces, Loki for logs, Prometheus for metrics):

```bash
bash integrations/observability/grafana/install.sh
kubectl port-forward -n observability svc/grafana-lgtm 3000:3000
```

Access Grafana at http://localhost:3000 (admin/admin). Change the default password if the cluster is accessible beyond localhost.

### Custom Metrics

KubeArchive exports these custom metrics:

- **CloudEvents counter:** Tracks events by type, resource type, and result (`insert`/`update`/`none`/`error`)
- **Resource updates counter:** Tracks delivery results
- **Workqueue metrics:** Depth, adds, latency, duration, retries, unfinished work (per resource watch in the operator)
- **Database pool metrics:** `db_sql_connection_open` with labels for pool status

### Prometheus Integration

For clusters with Prometheus Operator:

```bash
# Install ServiceMonitor
kubectl apply -f integrations/observability/prometheus-operator/

# Access Prometheus
kubectl port-forward -n prometheus-operator prometheus-prometheus-kube-prometheus-prometheus-0 9090
```

## Remote Debugging with Delve

For stepping through code with a debugger, use the debug deployment script. **Local development only** — this removes liveness/readiness probes, SecurityContext restrictions (seccomp, capabilities, read-only root filesystem), and resource limits to allow the debugger to attach:

```bash
# Deploy component in debug mode
bash test/debug/debug-deploy.sh api-server   # or: operator, sink

# Port-forward debug port
kubectl port-forward -n kubearchive svc/kubearchive-api-server 40000:40000

# Attach from IDE (VSCode or GoLand) — host: 127.0.0.1, port: 40000
```

After debugging, clean up:

```bash
hack/kubearchive-delete.sh && hack/kubearchive-install.sh
```

See the `kubearchive-local-setup` skill for full IDE configuration details.

## Error Patterns in the Codebase

When reading KubeArchive code, these patterns are used consistently:

- **`pkg/abort/abort.go`:** `abort.Abort(c, err, code)` — logs the error with context and returns a JSON error response. Used in all Gin handlers.
- **`pkg/database/errors/errors.go`:** `ErrResourceNotFound` — sentinel error for missing resources. Check with `errors.Is(err, dberrors.ErrResourceNotFound)`.
- **Error wrapping:** Uses `fmt.Errorf("context: %w", err)` for wrapping with `%w` so callers can use `errors.Is`/`errors.As`.
- **Multiple errors:** Uses `errors.Join()` for combining validation errors (e.g., environment variable checks).

## Health Endpoints Summary

| Component | Endpoint | Checks | Failure means |
|-----------|----------|--------|---------------|
| API Server | `/livez` | Always passes | Server is down entirely |
| API Server | `/readyz` | Database ping | Can't reach PostgreSQL |
| Sink | `/livez` | Always passes | Server is down entirely |
| Sink | `/readyz` | Database + K8s API | Can't write resources |
| Operator | `:8081/healthz` | controller-runtime ping | Manager not running |
| Operator | `:8081/readyz` | controller-runtime ping | Not ready for traffic |

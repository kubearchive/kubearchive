---
name: kubearchive-local-setup
description: Set up a local KubeArchive development environment on a Kind cluster, including dependencies, Tekton, API verification, database access, and remote IDE debugging with Delve. Use this skill whenever the user wants to set up, install, bootstrap, or initialize KubeArchive locally, create a Kind cluster for kubearchive, get the project running on their machine, debug kubearchive components remotely, or troubleshoot their local development environment. Also use when the user mentions "quick-install", "kind cluster", "local dev", "port-forward kubearchive", or "debug deploy".
---

# KubeArchive Local Setup

Guide Claude through setting up a complete KubeArchive local development environment on a Kind cluster, from cloning the repo to verifying the API and optionally attaching a remote debugger.

## Overview

The setup has three phases:
1. **Bootstrap** -- prerequisites, fork/clone, Kind cluster, install KubeArchive
2. **Verify** -- Tekton, port-forwarding, TLS, test users, API queries, DB access
3. **Debug** (optional) -- Delve remote debugging with VSCode or GoLand

Run each phase in order. If any step fails, diagnose before continuing -- later steps depend on earlier ones.

## Phase 1: Bootstrap

### 1.1 Check Prerequisites

Before starting, verify the required tools are installed. Run each check and report any missing tools to the user.

```bash
for tool in go git ko kubectl helm kind yq jq; do
  command -v "$tool" >/dev/null 2>&1 && echo "$tool: $(command -v $tool)" || echo "MISSING: $tool"
done
```

**Required versions:**
- `kubectl` >= v1.31
- `ko` >= v0.16 (required for `--debug` flag used in Delve debugging)

If any tools are missing, tell the user what to install and stop. Don't proceed with a partial toolchain.

### 1.2 Fork and Clone

The user should have their own fork of kubearchive. If they already have the repo cloned, skip this step.

```bash
git clone git@github.com:${YOUR_GITHUB_USERNAME}/kubearchive.git
cd kubearchive
git remote add upstream https://github.com/kubearchive/kubearchive.git
git remote set-url --push upstream no_push
```

The `no_push` URL on upstream prevents accidental pushes to the main repo.

### 1.3 Create Kind Cluster

```bash
kind create cluster
```

**Known issues:**
- **Rootless Podman:** If you get a systemd "Delegate" error, use:
  `systemd-run -p Delegate=yes --user --scope kind create cluster`
- **Multiple clusters / Podman Desktop:** If you get "No nodes found", set:
  `export KIND_CLUSTER_NAME=$(kind -q get clusters)`
- **"Too many open files":** Increase inotify limits:
  ```bash
  sudo sysctl fs.inotify.max_user_watches=524288
  sudo sysctl fs.inotify.max_user_instances=512
  ```

### 1.4 Configure ko and Install KubeArchive

Set ko to upload images to the Kind cluster:

```bash
export KO_DOCKER_REPO="kind.local"
```

Run the bootstrap installer -- this installs PostgreSQL (via CloudNative PG operator), cert-manager, and all KubeArchive components:

```bash
hack/quick-install.sh
```

This script calls `hack/kubearchive-install.sh` internally, which builds container images via `ko`, generates CRDs and RBAC, and deploys everything to the `kubearchive` namespace.

After install, verify pods are running:

```bash
kubectl get pods -n kubearchive
```

All pods should reach `Running` / `Ready` state within ~2 minutes.

## Phase 2: Verify

### 2.1 Install Tekton Pipelines

Tekton provides the pipeline resources that KubeArchive archives. The `latest` tag is fine for local development — for production, pin to a specific version (e.g., replace `latest` with `v0.62.0`):

```bash
kubectl apply --filename \
  https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
```

Wait for Tekton pods to be ready:

```bash
kubectl get pods --namespace tekton-pipelines --watch
```

### 2.2 Port-Forward the API Server

This occupies the terminal -- run it in a separate terminal or background it:

```bash
kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081
```

### 2.3 Extract TLS Certificate

The API server uses TLS. Extract the CA certificate to verify connections:

```bash
kubectl get -n kubearchive secrets kubearchive-api-server-tls \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
```

### 2.4 Create Test Users and Resources

Apply the test resources -- this creates a `test` namespace, service accounts with appropriate RBAC roles, and sample Tekton tasks/pipelines:

```bash
kubectl apply -f test/users/
```

This deploys:
- `test` namespace with a `default` service account that has `view` role
- Sample Tekton tasks (hello-world, goodbye) and a pipeline
- KubeArchiveConfig for the test namespace (archives PipelineRuns, TaskRuns, Pods, Jobs)

### 2.5 Query the API

Verify the API server responds:

```bash
curl -s --cacert ca.crt \
  -H "Authorization: Bearer $(kubectl create -n test token default)" \
  https://localhost:8081/apis/batch/v1/jobs | jq
```

A successful response returns a JSON list (possibly empty if no jobs have run yet).

**On OpenShift** with a local port-forward, you can use your user token and skip the CA cert. **Only use `-k` against localhost** — it disables server identity verification:

```bash
curl -k -H "Authorization: Bearer $(oc whoami --show-token)" \
  https://localhost:8081/apis/batch/v1/jobs
```

### 2.6 Check API Server Logs

Confirm the API server processed your request:

```bash
kubectl logs -n kubearchive -l app=kubearchive-api-server
```

### 2.7 Database Access (Optional)

To inspect the PostgreSQL database directly:

**On OpenShift:**

```bash
oc extract secret/kubearchive-database-credentials -n kubearchive --to /tmp/secret --confirm

oc debug -n kubearchive --image quay.io/fedora/postgresql-16
# Inside the debug pod:
psql -h <DATABASE_URL> -U <DATABASE_USER> -d <DATABASE_DB>
```

**On Kind**, port-forward the database service and use a local `psql` client, or exec into the database pod directly.

## Phase 3: Remote Debugging (Optional)

Use Delve to attach a remote debugger from your IDE. This replaces the normal deployment of a component with a debug-enabled build.

**Important:** Only debug one component at a time. After debugging, redeploy KubeArchive cleanly:
```bash
hack/kubearchive-delete.sh && hack/kubearchive-install.sh
```

### 3.1 Deploy in Debug Mode

The `test/debug/debug-deploy.sh` script builds the component with debug symbols (`ko build --debug`), removes probes and security constraints, and exposes port 40000 for Delve.

Choose one component:

```bash
# API Server
bash test/debug/debug-deploy.sh api-server

# Operator
bash test/debug/debug-deploy.sh operator

# Sink
bash test/debug/debug-deploy.sh sink
```

### 3.2 Port-Forward the Debug Port

**API Server** (also needs port 8081 for API access):
```bash
kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081 40000:40000
```

**Operator webhooks:**
```bash
kubectl port-forward -n kubearchive svc/kubearchive-operator-webhooks 40000:40000
```

### 3.3 Configure Your IDE

Read `references/ide-debug-config.md` for VSCode and GoLand configuration details.

The key settings: attach mode, port 40000, host 127.0.0.1, empty remotePath.

### 3.4 Generate Traffic

Set breakpoints in your IDE, then trigger the code path you want to debug:

**API Server** -- query the API (requires test users from Phase 2):
```bash
curl -s --cacert ca.crt \
  -H "Authorization: Bearer $(kubectl create -n test token default)" \
  https://localhost:8081/apis/batch/v1/jobs | jq
```

**Operator** -- apply a KubeArchiveConfig resource to trigger reconciliation.

## Updating After Code Changes

When you modify KubeArchive source code, redeploy without reinstalling dependencies:

```bash
hack/kubearchive-install.sh
```

To do a full clean reinstall:

```bash
hack/kubearchive-delete.sh
hack/quick-install.sh
```

Note: after uninstall/reinstall, you must re-apply any KubeArchiveConfig resources.

## Observability (Optional)

Install the Grafana LGTM stack (Tempo, Loki, Prometheus) for tracing and metrics:

```bash
bash integrations/observability/grafana/install.sh
kubectl port-forward -n observability svc/grafana-lgtm 3000:3000
```

Access at http://localhost:3000 (admin/admin). Change the default password if the cluster is accessible beyond localhost.

## Environment Variables Reference

| Variable | Purpose | Typical Value |
|----------|---------|---------------|
| `KO_DOCKER_REPO` | Container registry for ko builds | `kind.local` |
| `KO_DEFAULTBASEIMAGE` | Override base image (e.g., for debug tools) | `registry.redhat.io/rhel9/support-tools` |
| `KIND_CLUSTER_NAME` | Target Kind cluster (multi-cluster setups) | `$(kind -q get clusters)` |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `too many open files` / `leader election lost` | Low inotify limits | `sudo sysctl fs.inotify.max_user_watches=524288 && sudo sysctl fs.inotify.max_user_instances=512` |
| Kind fails with rootless Podman | Systemd cgroup delegation | `systemd-run -p Delegate=yes --user --scope kind create cluster` |
| "No nodes found for cluster" | Multiple Kind clusters or Docker mismatch | `export KIND_CLUSTER_NAME=$(kind -q get clusters)` |
| Pods stuck in `ImagePullBackOff` | `KO_DOCKER_REPO` not set or wrong | Verify `export KO_DOCKER_REPO="kind.local"` |
| API returns 401/403 | Missing test users or wrong token | Re-apply `kubectl apply -f test/users/` |
| Debug breakpoints not hit | Wrong remotePath or port not forwarded | Ensure `remotePath` is empty and port 40000 is forwarded |

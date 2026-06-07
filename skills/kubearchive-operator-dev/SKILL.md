---
name: kubearchive-operator-dev
description: Develop and modify the KubeArchive operator — reconcilers, CRDs, webhooks, CEL expressions, dynamic watches, RBAC management, and testing with envtest. Use this skill when someone is writing or modifying operator controllers, adding CRD fields, implementing webhooks, working with CEL filter expressions, managing dynamic resource watches, debugging reconciliation loops, or writing operator tests. Also trigger on "kubearchive reconciler", "KubeArchiveConfig CRD", "SinkFilter controller", "operator webhook", "CEL expression kubearchive", "envtest kubearchive", or "operator RBAC".
---

# KubeArchive Operator Development

The KubeArchive operator is a controller-runtime application with three reconcilers, five CRDs, and a dynamic resource watching system. This skill covers the patterns and conventions you need to modify or extend it.

## Codebase Layout

```
cmd/operator/
  main.go                           # Manager setup, reconciler + webhook registration
  api/v1/                           # CRD type definitions and webhooks
    kubearchiveconfig_types.go       # KubeArchiveConfig CRD spec/status
    kubearchiveconfig_webhook.go     # KAC validation + defaulting
    clusterkubearchiveconfig_types.go
    clusterkubearchiveconfig_webhook.go
    sinkfilter_types.go
    sinkfilter_webhook.go
    namespacevacuumconfig_types.go
    clustervacuumconfig_types.go
    groupversion_info.go             # Scheme registration
  internal/controller/
    kubearchiveconfig_controller.go  # KAC reconciler (RBAC, SinkFilter updates)
    clusterkubearchiveconfig_controller.go  # CKAC reconciler
    sinkfilter_controller.go         # Watch management, CEL eval, CloudEvents
    common.go                        # Shared helpers (RBAC, finalizers)
    configuration.go                 # Worker count config loading
    workqueue_metrics.go             # OpenTelemetry workqueue metrics
    suite_test.go                    # envtest setup
    *_test.go                        # Controller tests
```

## The Three Reconcilers

### KubeArchiveConfigReconciler

Handles namespace-scoped archival configurations. When a user creates a `KubeArchiveConfig` in their namespace, this reconciler:

1. Adds a finalizer (`kubearchive.org/finalizer`)
2. Updates the `SinkFilter` resource with the namespace's archival rules
3. Creates a `Role` in the user's namespace granting the sink `delete` permissions on archived resource types
4. Creates a `RoleBinding` binding the sink ServiceAccount to that Role
5. Creates a vacuum `ServiceAccount`, `Role`, and `RoleBinding` for retention enforcement

On deletion (finalizer runs):
1. Removes the namespace entry from the SinkFilter
2. Cleans up RBAC resources

**Key detail:** This reconciler also watches `ClusterKubeArchiveConfig` via `handler.EnqueueRequestsFromMapFunc` — when a CKAC changes, all KACs are re-reconciled to pick up cluster-wide rule changes.

### ClusterKubeArchiveConfigReconciler

Handles cluster-wide archival configurations. Similar to KAC but updates the `cluster` field of the SinkFilter instead of a namespace entry.

### SinkFilterReconciler

The most complex reconciler. It manages the actual resource watching system:

1. Reads the SinkFilter spec (aggregated from all KACs and CKACs)
2. Compares desired watches against running watches
3. Creates/updates/stops watch goroutines as needed
4. Each watch uses the Kubernetes dynamic client to watch a specific resource type
5. Watch events are evaluated against CEL expressions
6. Matching resources are wrapped in CloudEvents and sent to the sink
7. Manages a ClusterRole/ClusterRoleBinding granting the operator `get`/`list`/`watch` on all watched resource types

The watch system uses a worker pool pattern with rate-limiting workqueues. Worker counts per resource type are configurable via `/etc/kubearchive/config/resources.yaml`.

## CRD Types

### KubeArchiveConfig

```go
// cmd/operator/api/v1/kubearchiveconfig_types.go

type KubeArchiveConfigSpec struct {
    Resources []KubeArchiveConfigResource
}

type KubeArchiveConfigResource struct {
    Selector        APIVersionKind      // What to watch (Kind + APIVersion)
    ArchiveWhen     string              // CEL: when to archive (on create/update)
    DeleteWhen      string              // CEL: when to delete from cluster
    ArchiveOnDelete string              // CEL: archive when resource is deleted
    KeepLastWhen    *KeepLastWhenConfig  // Retention: keep N most recent
}
```

**Constraints enforced by webhook:**
- Resource name must be `kubearchive`
- Cannot be created in the `kubearchive` namespace
- All CEL expressions must be valid

### ClusterKubeArchiveConfig

Same structure as KAC but cluster-scoped. `KeepLastWhen` rules have a required `Name` field (must be unique) so namespace configs can override cluster rule counts.

### SinkFilter

```go
type SinkFilterSpec struct {
    Namespaces map[string][]KubeArchiveConfigResource  // Per-namespace rules
    Cluster    []ClusterKubeArchiveConfigResource      // Cluster-wide rules
}
```

Users don't create SinkFilters directly — the KAC and CKAC reconcilers manage it. Its name must be `sinkfilter` and it must live in the `kubearchive` namespace.

## Adding a New CRD Field

1. **Add the field** to the type struct in `cmd/operator/api/v1/<type>_types.go`
2. **Add validation** in the webhook file (`<type>_webhook.go`) if the field needs validation
3. **Add defaulting** in the mutating webhook if the field has a default value
4. **Run code generation:**
   ```bash
   make generate
   ```
   This regenerates DeepCopy methods, CRD YAML manifests, and RBAC manifests.
5. **Update the reconciler** if the new field changes reconciliation behavior
6. **Write tests** — unit test the webhook validation, integration test the reconciler

### Kubebuilder Markers

CRD behavior is controlled by marker comments:

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=kac;kacs
//+kubebuilder:validation:Required
//+kubebuilder:validation:Minimum=0
```

## Webhooks

Each CRD has a mutating webhook (defaulting) and a validating webhook.

### Mutating (Defaulter)

Implement the `Default()` method. Currently used to set default `SortBy` values in KeepLastWhen rules:

```go
func (r *KubeArchiveConfig) Default() {
    for i := range r.Spec.Resources {
        if r.Spec.Resources[i].KeepLastWhen != nil {
            for j := range r.Spec.Resources[i].KeepLastWhen.Keep {
                if r.Spec.Resources[i].KeepLastWhen.Keep[j].SortBy == "" {
                    r.Spec.Resources[i].KeepLastWhen.Keep[j].SortBy = "metadata.creationTimestamp"
                }
            }
        }
    }
}
```

### Validating

Implement `ValidateCreate()`, `ValidateUpdate()`, and `ValidateDelete()`. The KAC webhook validates:

- Resource name is `kubearchive`
- Namespace is not `kubearchive`
- All CEL expressions compile and are valid
- Duration function calls in CEL expressions use valid duration strings
- KeepLastWhen rules don't duplicate cluster rules
- Override references point to existing cluster rules with valid counts

### Adding a New Webhook

1. Add marker annotations to the type file
2. Implement the `Defaulter` and/or `CustomValidator` interfaces
3. Register in `main.go` with `mgr.GetWebhookServer()`
4. Run `make generate` to update webhook manifests

## CEL Expressions

CEL (Common Expression Language) expressions control when resources are archived. They receive the full Kubernetes resource as input and must return a boolean.

### How CEL Is Used

```go
// pkg/cel/cel.go
env, _ := cel.NewEnv(cel.Variable("self", cel.DynType))
ast, _ := env.Compile(expression)
prg, _ := env.Program(ast)
out, _ := prg.Eval(map[string]interface{}{"self": resource})
```

The resource is passed as `self` — a dynamic type representing the full unstructured Kubernetes resource.

### CEL Expression Examples

```yaml
archiveWhen: "true"                                    # always archive
archiveWhen: "self.status.phase == 'Succeeded'"        # archive completed pods
archiveWhen: "has(self.status.completionTime)"         # archive when completion time exists
deleteWhen: "self.status.phase == 'Failed'"            # delete failed resources
archiveOnDelete: "true"                                # archive on deletion
```

### Validating CEL in Webhooks

The webhook validation compiles CEL expressions at admission time — invalid expressions are rejected before the resource is created:

```go
func validateCELExpressions(resources []KubeArchiveConfigResource) field.ErrorList {
    for _, r := range resources {
        if r.ArchiveWhen != "" {
            if err := cel.CompileExpression(r.ArchiveWhen); err != nil {
                allErrors = append(allErrors, field.Invalid(...))
            }
        }
    }
}
```

## Dynamic Watch Management

The SinkFilterReconciler dynamically creates and manages Kubernetes watches. Understanding this is essential for modifying the operator's core archival logic.

### Watch Lifecycle

```
SinkFilter reconciled
    → findWatchesToCreate()   // new resource types to watch
    → findWatchesToUpdate()   // existing watches with changed filters
    → findWatchesToStop()     // resource types no longer in spec
```

Each watch runs in its own goroutine:

```go
func (r *SinkFilterReconciler) watchLoop(ctx context.Context, gvr schema.GroupVersionResource) {
    // Retry with exponential backoff (5s to 5m)
    for {
        watcher, _ := dynamicClient.Resource(gvr).Watch(ctx, opts)
        for event := range watcher.ResultChan() {
            queue.Add(event)  // rate-limited workqueue
        }
        // Watch expired or failed — reconnect with backoff
    }
}
```

Workers process events from the queue, evaluate CEL expressions, and publish CloudEvents:

```go
func (r *SinkFilterReconciler) processEvent(event watch.Event) {
    resource := event.Object.(*unstructured.Unstructured)
    
    switch event.Type {
    case watch.Added, watch.Modified:
        if cel.Evaluate(archiveWhen, resource) { publishCloudEvent(...) }
        if cel.Evaluate(deleteWhen, resource)  { deleteResource(...) }
    case watch.Deleted:
        if cel.Evaluate(archiveOnDelete, resource) { publishCloudEvent(...) }
    }
}
```

## RBAC Management

The operator dynamically creates RBAC resources. This is handled in `common.go`:

### Key Functions

- `policyRulesForResources()` — builds PolicyRules from a list of KubeArchiveConfigResources
- `reconcileRole()` / `reconcileClusterRole()` — create or update roles
- `reconcileRoleBinding()` / `reconcileClusterRoleBinding()` — create or update bindings
- `reconcileServiceAccount()` — create service accounts in target namespaces

### RBAC Annotations

Controllers declare their RBAC needs via kubebuilder annotations:

```go
//+kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
```

The `escalate` and `bind` verbs are intentionally required because the operator dynamically creates Roles for arbitrary resource types that users configure in their KubeArchiveConfig. Without `escalate`, the operator could only grant permissions it already holds — but it needs to grant the sink `delete` permissions on whatever resource types users choose to archive. **Do not copy this pattern into other controllers** unless they have the same dynamic RBAC requirement.

These generate the static ClusterRole in `config/`. Dynamic roles (for sink and vacuum) are created at runtime based on what resource types are being archived.

## Finalizers

All KubeArchiveConfig-family resources use the finalizer `kubearchive.org/finalizer` (defined in `common.go`).

The reconciliation pattern:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    resource := &v1.KubeArchiveConfig{}
    if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    if !resource.DeletionTimestamp.IsZero() {
        // Resource is being deleted — run cleanup
        // ... remove SinkFilter entries, RBAC resources
        controllerutil.RemoveFinalizer(resource, finalizerName)
        return ctrl.Result{}, r.Update(ctx, resource)
    }

    // Resource is being created/updated — ensure finalizer exists
    if controllerutil.AddFinalizer(resource, finalizerName) {
        if err := r.Update(ctx, resource); err != nil {
            return ctrl.Result{}, err
        }
    }
    // ... reconcile SinkFilter, RBAC, etc.
}
```

### Owner References

- Roles, RoleBindings, ServiceAccounts in the user's namespace are owned by the KubeArchiveConfig (garbage collected on KAC deletion)
- ClusterRoles and ClusterRoleBindings are NOT owned (cluster-scoped resources can't be owned by namespaced resources)
- The SinkFilter is NOT owned — it's a shared resource managed by multiple controllers

## Testing

### envtest Setup

Controller tests use envtest, which starts a local Kubernetes API server (etcd + kube-apiserver). The setup lives in `suite_test.go`:

```go
testEnv = &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "config", "crds")},
    ErrorIfCRDPathMissing: true,
}
cfg, _ = testEnv.Start()
k8sClient, _ = client.New(cfg, client.Options{Scheme: scheme.Scheme})
```

Tests create real Kubernetes resources and run reconcilers against them.

### Testing a Reconciler

```go
It("should reconcile KubeArchiveConfig", func() {
    // Create the resource
    kac := &v1.KubeArchiveConfig{...}
    Expect(k8sClient.Create(ctx, kac)).To(Succeed())

    // Run reconciliation
    reconciler := &KubeArchiveConfigReconciler{
        Client: k8sClient,
        Scheme: k8sClient.Scheme(),
        Mapper: k8sClient.RESTMapper(),
    }
    _, err := reconciler.Reconcile(ctx, reconcile.Request{
        NamespacedName: types.NamespacedName{Name: "kubearchive", Namespace: "test"},
    })
    Expect(err).NotTo(HaveOccurred())

    // Verify side effects
    sinkFilter := &v1.SinkFilter{}
    Expect(k8sClient.Get(ctx, sinkFilterKey, sinkFilter)).To(Succeed())
    Expect(sinkFilter.Spec.Namespaces).To(HaveKey("test"))
})
```

### Testing Webhooks

Webhook tests use `testify/assert` and test validation logic directly:

```go
func TestKubeArchiveConfigValidation(t *testing.T) {
    kac := &KubeArchiveConfig{
        ObjectMeta: metav1.ObjectMeta{Name: "wrong-name", Namespace: "test"},
        Spec: KubeArchiveConfigSpec{...},
    }
    _, err := kac.ValidateCreate()
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "must be named kubearchive")
}
```

### Running Tests

```bash
make test          # all unit tests including envtest
make test-race     # with race detector
make test-short    # skip slow tests
```

## Leader Election

Configured in `main.go`:

```go
LeaderElection:   enableLeaderElection,    // --leader-elect flag
LeaderElectionID: "e7a70c64.kubearchive.org",
LeaseDuration:    &leaseDuration,          // --leader-lease-duration (default: 15s)
```

Only one operator instance runs the reconcilers at a time. The lease lives in the kubearchive namespace.

## Common Patterns

### Adding a New Watched Resource Type

Users do this via KubeArchiveConfig — the operator handles watch creation automatically. If you need to modify how watches work, look at `SinkFilterReconciler.createWatchForGVR()`.

### Adding a New Controller

1. Create `cmd/operator/internal/controller/<name>_controller.go`
2. Implement the `Reconciler` interface with a `Reconcile()` method
3. Add `SetupWithManager()` to register watches
4. Call `SetupWithManager()` in `main.go`
5. Add RBAC annotations and run `make generate`

### Modifying CEL Evaluation

CEL compilation and evaluation lives in `pkg/cel/cel.go`. The CEL environment provides the resource as `self` (dynamic type). To add custom CEL functions or variables, modify the environment setup there.

### CloudEvent Types

The operator sends three types of CloudEvents to the sink:

| Event Type | Triggered By |
|-----------|-------------|
| `org.kubearchive.sinkfilters.resource.archive-when` | CEL `archiveWhen` matches on create/update |
| `org.kubearchive.sinkfilters.resource.delete-when` | CEL `deleteWhen` matches on create/update |
| `org.kubearchive.sinkfilters.resource.archive-on-delete` | CEL `archiveOnDelete` matches on deletion |

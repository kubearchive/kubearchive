# Advanced Webhook Readiness Check

## Overview

The KubeArchive operator now includes a sophisticated readiness probe that verifies the webhook service is actually responding on port 443, replacing the default `healthz.Ping` function. This implementation is designed for the advanced multi-controller operator architecture.

## Architecture Integration

### Advanced Operator Features
This implementation works with the enhanced operator that includes:
- **Multiple Controllers**: KubeArchiveConfig, ClusterKubeArchiveConfig, and SinkFilter controllers
- **API v1**: Upgraded from v1alpha1 with full webhook validation
- **Centralized Constants**: Uses `pkg/constants` package for configuration
- **Advanced Caching**: Sophisticated cache configuration with selective resource watching
- **Enhanced Observability**: Integrated with OpenTelemetry and configurable metrics

### Centralized Configuration

The webhook readiness check leverages the centralized constants package:

```go
// pkg/constants/constants.go
const (
    KubeArchiveOperatorWebhooksName = "kubearchive-operator-webhooks"
    // ... other constants
)

var (
    KubeArchiveNamespace string // Dynamically set from KUBEARCHIVE_NAMESPACE
)
```

## Implementation Details

### Enhanced Readiness Check

The `webhookReadinessCheck` function provides:

1. **Constants Integration**: Uses `constants.KubeArchiveOperatorWebhooksName` and `constants.KubeArchiveNamespace`
2. **Dynamic Namespace**: Automatically uses the correct namespace from environment variables
3. **TLS Verification**: Tests actual TLS connectivity to webhook service on port 443
4. **Timeout Protection**: 5-second timeout prevents hanging readiness checks
5. **Comprehensive Logging**: Debug and warning logs for operational visibility

### Service Discovery

```go
serviceAddr := fmt.Sprintf("%s.%s.svc.cluster.local:%s",
    constants.KubeArchiveOperatorWebhooksName, 
    constants.KubeArchiveNamespace, 
    webhookServicePort)
// Results in: kubearchive-operator-webhooks.kubearchive.svc.cluster.local:443
```

### Configuration Constants

```go
const (
    otelServiceName               = "kubearchive.operator"
    webhookServicePort            = "443"
    webhookConnectionTimeout      = 5 * time.Second
)
```

## Multiple Webhook Support

The enhanced operator supports multiple webhook configurations:
- **ClusterKubeArchiveConfig** webhooks
- **KubeArchiveConfig** webhooks  
- **SinkFilter** webhooks
- **NamespaceVacuumConfig** webhooks
- **ClusterVacuumConfig** webhooks

The readiness check ensures all webhook infrastructure is available before any webhook processing begins.

## Problem Solved

**Before:**
- Operator reported "ready" even when webhook service wasn't responding
- Manual verification required for multi-controller deployments
- KubeArchive resource applications would fail if applied too early
- No visibility into webhook service status across multiple resource types

**After:**
- Readiness check verifies actual webhook service availability
- Works seamlessly with all controller types (KAC, CKAC, SinkFilter, etc.)
- All KubeArchive resources can be applied immediately after readiness
- Clear operational visibility into webhook infrastructure status

## Testing

### 1. Verify Enhanced Readiness

```bash
# Check that the readiness endpoint performs webhook verification
kubectl port-forward -n kubearchive deployment/kubearchive-operator 8081:8081
curl http://localhost:8081/readyz
```

### 2. Test Multi-Controller Deployment

Deploy the operator with all controllers and monitor readiness:

```bash
# Deploy advanced operator
kubectl apply -f kubearchive-operator.yaml

# Watch readiness - should stay NotReady until webhooks are up
kubectl get pods -n kubearchive -w

# Verify all controllers are registered
kubectl logs -n kubearchive deployment/kubearchive-operator | grep "registering.*controller"
```

### 3. Test Resource Application

The readiness check should prevent issues when applying any resource type:

```bash
# Deploy operator and wait for readiness
kubectl apply -f kubearchive-operator.yaml
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n kubearchive

# All resource types should work immediately
kubectl apply -f kubearchive-config.yaml           # KubeArchiveConfig
kubectl apply -f cluster-kubearchive-config.yaml   # ClusterKubeArchiveConfig  
kubectl apply -f sink-filter.yaml                  # SinkFilter
kubectl apply -f namespace-vacuum-config.yaml      # NamespaceVacuumConfig
kubectl apply -f cluster-vacuum-config.yaml        # ClusterVacuumConfig
```

## Benefits

1. **Multi-Controller Support**: Works with all KubeArchive controller types
2. **Centralized Configuration**: Leverages constants package for maintainability
3. **Advanced Architecture Ready**: Designed for sophisticated operator deployments
4. **Enhanced Observability**: Integrates with advanced logging and metrics
5. **Namespace Awareness**: Uses dynamic namespace configuration
6. **Production Ready**: Supports complex deployment scenarios

## Troubleshooting

### Advanced Debugging

Enable detailed logging to see readiness check information:

```bash
# View operator logs with advanced filtering
kubectl logs -n kubearchive deployment/kubearchive-operator -f | grep -E "(webhook|readiness|controller)"
```

### Multi-Controller Issues

1. **Controller Registration**: Check that all three controllers are registered
2. **Webhook Conflicts**: Verify webhook configurations don't conflict
3. **Resource Conflicts**: Ensure RBAC permissions are correct for all controllers
4. **Cache Issues**: Check cache configuration for resource watching

### Log Messages

Look for advanced operator log messages:
- `"registering SinkFilter controller"` - SinkFilter controller startup
- `"Checking webhook readiness"` - Readiness check execution
- `"Webhook readiness check passed"` - Successful verification
- `"unable to create controller"` - Controller registration issues
- `"unable to create webhook"` - Webhook setup problems

## Architecture Benefits

This implementation scales with the advanced operator architecture:
- **Supports all controller types** without modification
- **Uses centralized configuration** for easy maintenance
- **Integrates with advanced caching** for optimal performance
- **Works with enhanced RBAC** for multi-controller permissions
- **Supports dynamic namespace configuration** for flexible deployments


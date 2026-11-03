# Integration Tests

## Prerequisites

* Install the normal KubeArchive development tools, see the [README](../README.md).
* Install [ko](https://ko.build/install/).

## Run the tests

The integration tests require KubeArchive and its database, certificate manager,
and logging stack to be installed in the cluster selected by your kubeconfig.
Creating a cluster alone is not sufficient. Use the same setup as the integration
test workflow:

```bash
kind create cluster
export KO_DOCKER_REPO=kind.local
bash hack/quick-install.sh
bash integrations/logging/loki/install.sh --vector
go test -v -count=1 ./test/... -tags=integration -parallel 4 -timeout 15m
```

If the Kind cluster already exists, skip `kind create cluster` and verify that
`kubectl config current-context` selects it before running the installers.

The concurrency limit keeps a single-node local cluster from being overloaded by
the parallel tests. Increase `-parallel` if your cluster has sufficient capacity.

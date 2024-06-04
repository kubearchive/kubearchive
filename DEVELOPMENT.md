# Development

This document helps you to setup a development environment so you can contribute
to KubeArchive. It also contain instructions to run integration tests, remote IDE
debugging and other processes.

## Requisites

Install these tools:

1. [`go`](https://golang.org/doc/install)
1. [`git`](https://help.github.com/articles/set-up-git/)
1. [`ko`](https://github.com/google/ko)
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
1. [`helm`](https://helm.sh/docs/intro/install/)
1. [`podman`](https://podman.io/docs/installation)
1. [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/)

## Creating a fork repository

1. Create your fork of [KubeArchive](https://github.com/kubearchive/kubearchive)
  following [this guide](https://help.github.com/articles/fork-a-repo/).
1. Clone it to your computer:
  
    ```bash
    git clone git@github.com:${YOUR_GITHUB_USERNAME}/kubearchive.git
    cd kubearchive
    git remote add upstream https://github.com/kubearchive/kubearchive.git
    git remote set-url --push upstream no_push
    ```

## Create a cluster

1. Set up a Kubernetes cluster with KinD:
    ```bash
    kind create cluster
    ```

1. Set up `ko` to upload images to the KinD cluster, or any other registry you
  want to use:
    ```bash
    export KO_DOCKER_REPO="kind.local"
    ```

## Install KubeArchive

1. Use Helm to install KubeArchive:
    ```bash
    helm install kubearchive charts/kubearchive \
        --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
        --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink)
    ```
1. Check that the KubeArchive deployments are Ready:
    ```bash
    kubectl get -n kubearchive deployments
    ```

## Update KubeArchive

After you make changes to the code use Helm to redeploy KubeArchive:

```bash
helm upgrade kubearchive charts/kubearchive \
    --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
    --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink)
```

# Uninstall KubeArchive

```bash
helm uninstall kubearchive
```

## Generate activity on the KubeArchive sink

By default KubeArchive listens to `Event`s in the `test` namespace.

1. Generate some activity creating a pod:
    ```bash
    kubectl run -n test busybox --image=busybox
    ```
1. Follow the logs on the KubeArchive sink:
    ```bash
    kubectl logs -n kubearchive -l app=kubearchive-sink -f
    ```

## Forward the KubeArchive API to localhost

1. Use `kubectl` to port forward, this will keep the terminal occupied:
    ```bash
    kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081
    ```
1. On a new terminal, use `curl` or your browser to perform a query:
    ```bash
    curl localhost:8081/apis/apps/v1/deployments
    ```
1. Check the new logs on the KubeArchive API:
    ```bash
    kubectl logs -n kubearchive -l app=kubearchive-api-server
    ```

## Running integration tests

Use `go test` to run the integration test suite:
```bash
go test -v ./... -tags=integration
```

## Remote IDE debugging

Use [delve](https://golangforall.com/en/post/go-docker-delve-remote-debug.html)
to start a debugger to which attach from your IDE.

1. Deploy the chart with `ko` and `helm` in debug mode using an image with `delve`:
   ```bash
   helm install -n default [deployment name] charts/kubearchive \ 
   --set apiServer.debug=true \
   --set-string apiServer.image=$(KO_DEFAULTBASEIMAGE=gcr.io/k8s-skaffold/skaffold-debug-support/go:latest \
   ko build --disable-optimizations github.com/kubearchive/kubearchive/cmd/api) \
   --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink)
   ```

1. Forward the ports 8081 and 40000 from the Pod directly:
   ```bash
   kubectl port-forward \
   $(kubectl get -n kubearchive pods --no-headers -o custom-columns=":metadata.name" | grep api-server) \
   8081:8081 40000:40000
   ```
1. Enable breakpoints in your IDE.
1. Connect to the process using the port 40000:
   * [VSCode instructions](https://golangforall.com/en/post/go-docker-delve-remote-debug.html#visual-studio-code)
   * [Goland instructions](https://golangforall.com/en/post/go-docker-delve-remote-debug.html#goland-ide)
1. Query the API using `curl` or your browser:
   ```bash
   curl localhost:8081/apis/apps/v1/deployments
   ```

## Known issues

1. Using KinD and podman. If you get this error:
    ```
    ERROR: failed to create cluster: running kind with rootless provider requires
    setting systemd property "Delegate=yes", see https://kind.sigs.k8s.io/docs/user/rootless/
    ```
    try creating the cluster with this command:
    ```bash
    systemd-run -p Delegate=yes --user --scope kind create cluster
    ```

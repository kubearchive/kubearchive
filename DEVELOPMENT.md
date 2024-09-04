# Development

This document helps you to setup a development environment so you can contribute
to KubeArchive. It also contain instructions to run integration tests, remote IDE
debugging and other processes.

## Requisites

Install these tools:

1. [`go`](https://golang.org/doc/install)
1. [`git`](https://help.github.com/articles/set-up-git/)
1. [`ko`](https://github.com/google/ko) ( >v0.16 if using `dlv` for interactive debugging )
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
1. [`helm`](https://helm.sh/docs/intro/install/)
1. [`podman`](https://podman.io/docs/installation)
1. [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/)
1. [`yq`](https://github.com/mikefarah/yq/)

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

1. Install knative-eventing core and cert-manager, wait for them to be ready, and enable new-apiserversource-filters:
    ```bash
    export CERT_MANAGER_VERSION=v1.9.1
    export KNATIVE_EVENTING_VERSION=v1.15.0

    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
    kubectl apply -f https://github.com/knative/eventing/releases/download/knative-${KNATIVE_EVENTING_VERSION}/eventing-core.yaml
    kubectl rollout status deployment --namespace=cert-manager --timeout=30s
    kubectl rollout status deployment --namespace=knative-eventing --timeout=30s

    kubectl apply -f - << EOF
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: config-features
      namespace: knative-eventing
      labels:
        eventing.knative.dev/release: devel
        knative.dev/config-propagation: original
        knative.dev/config-category: eventing
    data:
      new-apiserversource-filters: enabled
    EOF
    ```

1. Generate operator code:
    ```bash
    cmd/operator/generate.sh
    ```

## Install KubeArchive

1. Use Helm to install KubeArchive:
   ```bash
   helm install kubearchive charts/kubearchive --create-namespace -n kubearchive \
       --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
       --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
       --set-string operator.image=$(ko build github.com/kubearchive/kubearchive/cmd/operator)
   ```
1. Check that the KubeArchive deployments are Ready:
   ```bash
   kubectl get -n kubearchive deployments
   ```

1. List the deployed helm chart
   ```bash
   helm list -n kubearchive
   ```

## Update KubeArchive

After you make changes to the code use Helm to redeploy KubeArchive:

```bash
helm upgrade kubearchive charts/kubearchive -n kubearchive \
    --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
    --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
    --set-string operator.image=$(ko build github.com/kubearchive/kubearchive/cmd/operator)
```

## Uninstall KubeArchive

```bash
helm uninstall -n kubearchive kubearchive
```

## Initialize the database

1. In a new terminal tab create a port-forward.
   ```bash
   kubectl port-forward -n kubearchive svc/kubearchive-database 5432:5432
   ```
2. Populate the database with test objects.
   ```bash
   go run database/init_db.go
   ```

## Generate activity on the KubeArchive sink

By default, KubeArchive listens to `Event`s in the `test` namespace.

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
1. Get the Certificate Authority (CA) from the `kubearchive-api-server-tls` secret:
    ```bash
    kubectl get -n kubearchive secrets kubearchive-api-server-tls -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
    ```

1. **[ Optional ]** Create a service account with a specific role to test the REST API.
    This Helm chart already provides `kubearchive-test-sa` with `view` privileges for testing purposes.

1. On a new terminal, use `curl` or your browser to perform a query:
    ```bash
    curl -s --cacert ca.crt -H "Authorization: Bearer $(kubectl create token kubearchive-test -n kubearchive)" \
    https://localhost:8081/apis/batch/v1/jobs | jq
    ```

1. Check the new logs on the KubeArchive API:
    ```bash
    kubectl logs -n kubearchive -l app=kubearchive-api-server
    ```

## Use the KubeArchive CLI

1. Use `kubectl` to port forward, this will keep the terminal occupied:
    ```bash
    kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081
    ```

1. Get the Certificate Authority (CA) from the `kubearchive-api-server-tls` secret:
    ```bash
    kubectl get -n kubearchive secrets kubearchive-api-server-tls -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
    ```

1. Run the CLI:
    ```bash
    go run cmd/kubectl-archive/main.go get batch/v1/jobs --token $(kubectl -n kubearchive create token kubearchive-test)
    ```

1. Generate a new job, and run again:
    ```bash
    kubectl create job my-job --image=busybox
    go run cmd/kubectl-archive/main.go get batch/v1/jobs --token $(kubectl -n kubearchive create token kubearchive-test)
    ```

## Running integration tests

Use `go test` to run the integration test suite:

```bash
go test -v ./test/integration -tags=integration
```

## Running ko builds with extra debugging tools
By default `ko` uses a lightweight image to run go programs.
The image can be [overridden](https://ko.build/configuration/#overriding-base-images) with
the `defaultBaseImage` parameter of the `.ko.yaml` configuration file or through the
environment variable  `KO_DEFAULTBASEIMAGE`.

An example of an image with a set of debugging tools already installed that can be used is
`registry.redhat.io/rhel9/support-tools`

```bash
export KO_DEFAULTBASEIMAGE=registry.redhat.io/rhel9/support-tools
```

**[NOTE]**: Using this images usually needs the pod property `runAsNonRoot` set to `false`.

## Remote IDE debugging

Use [delve](https://golangforall.com/en/post/go-docker-delve-remote-debug.html)
to start a debugger to which attach from your IDE.

1. Deploy the chart with `ko` and `helm` in debug mode using an image with `delve`:
   Specify the [component].debug variable to `true` and update the image accordingly.

   * Deployment to debug the API:
   ```bash
   helm install kubearchive charts/kubearchive --create-namespace -n kubearchive \
     --set apiServer.debug=true \
     --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api --debug) \
     --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
     --set-string operator.image=$(ko build github.com/kubearchive/kubearchive/cmd/operator)
   ```
   * Deployment to debug the Operator:
   ```bash
   helm install kubearchive charts/kubearchive --create-namespace -n kubearchive \
     --set operator.debug=true \
     --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
     --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
     --set-string operator.image=$(ko build github.com/kubearchive/kubearchive/cmd/operator --debug)
   ```

1. Forward the port 40000 from the service that you want to debug:

   * To debug the API we also need the 8081 port for exposing the API Server:
   ```bash
   kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081 40000:40000
   ```
   * Debug the operator webhooks:
   ```bash
   kubectl port-forward -n kubearchive svc/kubearchive-operator-webhooks 40000:40000
   ```

1. Enable breakpoints in your IDE.
1. Connect to the process using the port 40000:
    * [VSCode instructions](https://golangforall.com/en/post/go-docker-delve-remote-debug.html#visual-studio-code)
    * [Goland instructions](https://golangforall.com/en/post/go-docker-delve-remote-debug.html#goland-ide)
1. Generate traffic:
    * API: Query the API using `curl` or your browser:
   ```bash
   curl -s --cacert ca.crt -H "Authorization: Bearer $(kubectl create token kubearchive-test -n kubearchive)" \
   https://localhost:8081/apis/batch/v1/jobs | jq
   ```
   * Operator: Deploy the test resources that already include a KubeArchiveConfig Custom Resource

## Enabling Telemetry

We use the Grafana Labs Stack (Grafana, Tempo, Loki and Prometheus) for observability on development.
Specifially we use the
[LGTM Docker container](https://github.com/grafana/docker-otel-lgtm)
which also includes an OpenTelemetry Collector.
As some dependencies use the Zipkin format to send traces, we are using the
Collector's zipkin receiver.

**Note**: KubeArchive sends traces and metrics to an intermediate OpenTelemetry
Collector, which sends the data to the LGTM's Collector.

1. Create the following ConfigMap to enable telemetry for Knative Eventing.
    ```bash
    kubectl apply -f - <<EOF
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: config-tracing
      namespace: knative-eventing
    data:
      backend: "zipkin"
      zipkin-endpoint: "http://grafana-lgtm.kubearchive.svc.cluster.local:9411"
      sample-rate: "1"
    EOF
    ```
    **Note**: Knative's APIServerSource created before applying this change do not emit traces. Recreate
    the KubeArchiveConfig to trigger the recreation of the APIServerSource.
1. Install or upgrade KubeArchive adding the following option to the Helm command:
    ```bash
    --set "integrations.observability.enabled=true"
    ```
    This extra flag makes Helm deploy all necessary resources for observability
    and configures the environment variables on the KubeArchive components.
1. Forward the Grafana UI port to localhost:
    ```bash
    kubectl port-forward -n kubearchive svc/grafana-lgtm 3000:3000 &
    ```
1. Open [http://localhost:3000](http://localhost:3000) in your browser, use `admin`
as username and `admin` as password. In the sidebar go to the Explore section to
start exploring metrics.

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
1. Using KinD and Podman Desktop. If you get this error:
   ```
   Error: failed to publish images: error publishing
   ko://github.com/kubearchive/kubearchive/cmd/api: no nodes found for cluster "kind"
   ```
   expose the `KIND_CLUSTER_NAME` env variable with the appropriate name of the kind cluster:
   ```bash
   export KIND_CLUSTER_NAME=$(kind -q get clusters)
   ```
   **NOTE**: In case you have more than one kind cluster running, manually set the proper one

# Development

This document helps you to setup a development environment so you can contribute
to KubeArchive. It also contain instructions to run integration tests, remote IDE
debugging and other processes.

## Requisites

Install these tools:

1. [`go`](https://golang.org/doc/install)
1. [`git`](https://help.github.com/articles/set-up-git/)
1. [`ko`](https://github.com/google/ko) ( > v0.16 if using `dlv` for interactive debugging )
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) ( >= v1.31 )
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

## Install KubeArchive, its dependencies, and initialize the database
   ```bash
   hack/quick-install.sh
   ```

## Update KubeArchive

After you make changes to the code use the script to redeploy KubeArchive:

```bash
   hack/kubearchive-install.sh
```

## Uninstall KubeArchive

```bash
   hack/kubearchive-delete.sh
```

**[NOTE]**: If KubeArchive is uninstalled and re-installed, all `KubeArchiveConfig` resources must be re-applied.

## Generate activity on the KubeArchive sink

1. Install the CronJob log generator
    ```bash
    test/log-generators/cronjobs/install.sh
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

1. Create a service account with a specific role to test the REST API.
   You can use the `default` user with `view` privileges provided for the `test` namespace in `test/users/test-user.yaml`.
    ```bash
    kubectl apply -f test/users/
    ```

1. On a new terminal, use `curl` or your browser to perform a query:
    ```bash
    curl -s --cacert ca.crt -H "Authorization: Bearer $(kubectl create -n test token default)" \
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
    go run cmd/kubectl-archive/main.go get batch/v1 jobs --token $(kubectl create -n test token default)
    ```
   **NOTE**: For this to work the `test/users/test-user.yaml` must be applied.

1. Generate a new job, and run again:
    ```bash
    kubectl create job my-job --image=busybox
    go run cmd/kubectl-archive/main.go get batch/v1 jobs --token $(kubectl create -n test token default)
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

1. Run the script `test/debug/debug-deploy.sh` with one of the following values: `operator`, `sink`, `api-server`
    * Deployment to debug the API:
    ```bash
    bash test/debug/debug-deploy.sh api-server
    ```
    * Deployment to debug the Operator:
    ```bash
    bash test/debug/debug-deploy.sh operator
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

   **NOTE**: For this to work the `test/users/test-user.yaml` must be applied.
   ```bash
   curl -s --cacert ca.crt -H "Authorization: Bearer $(kubectl create -n test token default)" \
   https://localhost:8081/apis/batch/v1/jobs | jq
   ```
   * Operator: Deploy the test resources that already include a KubeArchiveConfig Custom Resource

**NOTE**: Debug just one component at once. After debugging a component, redeploy KubeArchive
with `hack/kubearchive-delete.sh` and `hack/kubearchive-install.sh`

## Enabling Telemetry

We use the Grafana Labs Stack (Grafana, Tempo, Loki and Prometheus) for observability on development.
Specifially we use the
[LGTM Docker container](https://github.com/grafana/docker-otel-lgtm)
which also includes an OpenTelemetry Collector.
As some dependencies use the Zipkin format to send traces, we are using the
Collector's zipkin receiver.

**Note**: KubeArchive sends traces and metrics to an intermediate OpenTelemetry
Collector, which sends the data to the LGTM's Collector.

1. After installing KubeArchive, run:
    ```bash
    bash integrations/observability/grafana/install.sh
    ```
    **Note**: Knative's APIServerSource created before applying this change do not emit traces. Recreate
    the KubeArchiveConfig to trigger the recreation of the APIServerSource.
1. Forward the Grafana UI port to localhost:
    ```bash
    kubectl port-forward -n observability svc/grafana-lgtm 3000:3000 &
    ```
1. Open [http://localhost:3000](http://localhost:3000) in your browser, use `admin`
as username and `admin` as password. In the sidebar go to the Explore section to
start exploring metrics.

## Logging

KubeArchive currenty has integrations for both Elasticsearch and Splunk. The sections
below detail how to install each of those logging systems in a development environment.
When the installation is complete, `Pod` logs will be sent to the logging system automatically.

Once a logging system is installed, KubeArchive needs to be configured to generate log
URLs for it.  This is all detailed in the
[KubeArchive documentation for logging integrations](https://kubearchive.github.io/kubearchive/main/integrations/logging.html). See this documentation for instructions and examples for configuring KubeArchive to generate
log URLs.

* [ElasticSearch](./integrations/logging/elasticsearch/README.md)
* [Splunk](./integrations/logging/splunk/README.md)
* [Datadog](./integrations/logging/datadog/README.md)

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

1. Deploying the operator (for example using the `hack/quick-install.sh` script), the `Deployment` doesn't reach `Ready` state:
    ```
    Waiting for deployment "kubearchive-operator" rollout to finish: 0 of 1 updated replicas are available...
    error: timed out waiting for the condition
    ```
    And in the logs of the `kubearchive-operator` you see the following ERROR:
    ```
     ❯ kubectl logs -n kubearchive deploy/kubearchive-operator --tail=5
     2024-09-20T08:45:35Z    ERROR   error received after stop sequence was engaged  {"error": "leader election lost"}
     sigs.k8s.io/controller-runtime/pkg/manager.(*controllerManager).engageStopProcedure.func1
     sigs.k8s.io/controller-runtime@v0.19.0/pkg/manager/internal.go:512
     2024-09-20T08:45:35Z    ERROR   setup   problem running operator        {"error": "too many open files"}
     main.main
     github.com/kubearchive/kubearchive/cmd/operator/main.go:154
     runtime.main
     runtime/proc.go:271
    ```
    Run the following command:
    ```bash
    sudo sysctl fs.inotify.max_user_watches=524288
    sudo sysctl fs.inotify.max_user_instances=512
    ```

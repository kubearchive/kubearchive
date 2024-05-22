# kubearchive

## Overview
KubeArchive is a utility that stores Kubernetes resources off of the
Kubernetes cluster. This enables users to delete those resources from
the cluster without losing the information contained in those resources.
KubeArchive will provide an API so users can retrieve stored resources
for inspection.

The main users of KubeArchive are projects that use Kubernetes resources
for one-shot operations and want to inspect those resources in the long-term.
For example, users using Jobs on Kubernetes that want to track the success
rate over time, but need to remove completed Jobs for performance/storage
reasons, would benefit from KubeArchive. Another example would be users
that run build systems on top of Kubernetes (Shipwright, Tekton) that use
resources for one-shot builds and want to keep track of those builds over time.

## Requirements
To deploy kubearchive locally you need to install the following:
* podman
* jq
* helm
* kubectl
* kind
* cosign

On fedora, install podman, jq, and helm with this command:
```bash
sudo dnf install podman jq helm
```
Otherwise, follow the [podman](https://podman.io/docs/installation), [jq](https://jqlang.github.io/jq/download/), and [helm](https://helm.sh/docs/intro/install/) install instructions.

Follow the [kubernetes](https://kubernetes.io/docs/tasks/tools/#kubectl), [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), and [cosign](https://docs.sigstore.dev/system_config/installation/) install instructions.

## Local Deployment
Create a cluster using kind.
```bash
kind create cluster
```
By default the cluster name is `kind`. You can choose a name by using the `--name` flag.
If you are still getting this error after following the instructions [here](https://kind.sigs.k8s.io/docs/user/rootless/)
```
ERROR: failed to create cluster: running kind with rootless provider requires setting systemd property "Delegate=yes", see https://kind.sigs.k8s.io/docs/user/rootless/
```
try creating the cluster with this command:
```bash
systemd-run -p Delegate=yes --user --scope kind create cluster
```

After the cluster is created, run the kubectl command printed by kind to set your kube context to the kind cluster.

Verfiy the image signatures for knative:
```bash
curl -sSL https://github.com/knative/serving/releases/download/knative-v1.13.1/serving-core.yaml \
  | grep 'gcr.io/' | awk '{print $2}' | sort | uniq \
  | xargs -n 1 \
    cosign verify -o text \
      --certificate-identity=signer@knative-releases.iam.gserviceaccount.com \
      --certificate-oidc-issuer=https://accounts.google.com
```

Finally run the helm chart to delploy kubearchive:
```bash
helm install [deployment name] charts/kubearchive
```
You can use the `-g` flag to have helm generate a deployment name for you.

Run this command remove the kubearchive deployment:
```bash
helm uninstall [deployment name] -n default
```
**NOTE**: This assumes you installed the helm chart under the `default` namespace.

### Build images with `ko`

The following components of kubearchive have images that can be built locally with `ko`:
* kubearchive-api-server
* kubearchive-sink

`ko` can be installed with the following command
```bash
go install github.com/google/ko@latest
```
or follow the [ko install instructions](https://ko.build/install).

To use ko with kind we need to set up the following environment variables:
```bash
export KO_DOCKER_REPO=kind.local
export KIND_CLUSTER_NAME=<kind_cluster_name> # Defaults to "kind"
```
Then run the `helm install` command as follows:
```bash
helm install -n default [deployment name] charts/kubearchive \
--set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
--set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink)
```

> **_NOTE_**: To upgrade the deployment instead of running it from scratch use:
> ```bash
> helm upgrade -n default --reuse-values -f charts/kubearchive/values.yaml \
> --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
> --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
> [deployment name] charts/kubearchive --version 0.0.1
> ```
> This is needed if changes are made to the code.

## Kubearchive Helm Chart

The kubearchive helm chart deploys the following:
* Namespace named `kubearchive`
* ClusterRole named `kubearchive`
* ClusterRoleBinding named `kubearchive`
* Service Account named `kubearchive` in the `kubearchive` namespace
* ApiServerSource named `api-server-source` in the `kubearchive` namespace
* Deployment and Service for `kubearchive-sink` in the `kubearchive` namespace
* (optionally) Namespace named `test`

The settings of each resource are the same as in
[Create an ApiServerSource object](https://knative.dev/docs/eventing/sources/apiserversource/getting-started/#create-an-apiserversource-object)
tutorial of the knative docs.

### ApiServerSource Configuration
The ApiServerSource deployed by this helm chart uses the `kubearchive` service account to watch resources
on the cluster. By default, it is deployed to watch for events. The `ClusterRole` and `ClusterRoleBinding`
by default give the kubearchive service account permissions to `get`, `list`, and `watch` `Events` resources cluster-wide.
The ApiServerSource is deployed by default to only listen for events in namespaces with the label `kubearchive: watch`.
The `test` namespace, if created, has that label applied. The resources that the ApiServerSource listens for can be
changed by running the helm chart with `kubearchive.role.rules[0].resources` and `apiServerSource.resources` overridden.
`kubearchive.role.rules[0].resources` expects that the resource type(s) list are all lowercase and plural. If one
or more of the resources in `kubearchive.role.rules[0].resources` is not in the kubernetes core API group, then
`kubearchive.role.rules[0].apiGroups` must be overridden as well to include all API groups that contain all the
resources that you are interested in. `apiServerSource.resources` is a list where each item includes the `kind` and
`apiVersion` of the resource.

### kubearchive-sink
An ApiServerSource requires a sink that it can send cloud events to. The image can be hardcoded or ko can be used to
dynamically build and deploy the containers with the appropriate image.

The kubearchive-sink logs are viewable with this command
command:
```bash
kubectl logs --namespace=kubearchive -l app=kubearchive-sink --tail=1000
```

### Create an event
Run a pod to create an event and remove it to create another one:
```bash
kubectl run busybox --image=busybox --namespace=test --restart=Never -- ls
kubectl --namespace delete pod busybox
```

Check the events in the logs of the sink:
```bash
kubectl logs --namespace=kubearchive -l app=kubearchive-sink --tail=100
```

### Watch other resources
More resources can be watched appart from `Events`.
Here is an example of the tweaks needed to add `Pods` and `ConfigMaps` to the watched resources:
1. In the file `values.yaml` add the resources to be watched under `apiServerSource.resources`:
    ```yaml
    # ...
    apiServerSource:
      # ...
      resources:
        - apiVersion: v1
          kind: Event
        - apiVersion: v1
          kind: ConfigMap
        - apiVersion: v1
          kind: Pod
    ```
2. In the same file, add the resources names in the rules for the role permissions:
   ```yaml
   # ...
   kubearchive:
      # ...
      role:
        rules:
          - apiGroups:
            resources:
              - events
              - configmaps
              - pods
            # ...
   ```
3. Apply the new changes with `helm`
   ```bash
   helm upgrade -n default --reuse-values -f charts/kubearchive/values.yaml [deployment name] charts/kubearchive --version 0.0.1
   ```
   > **_NOTE:_**  A tweak is needed to this command for successfully deploying the api-server.
   Check the section [Remote debugging API Server from the IDE](#remote-debugging-api-server-from-the-ide).

4. Test the new cloud events sent to the sink by creating and deleting a `configmap` and a `pod`
   ```bash
   kubectl -n test create configmap my-config --from-literal=key1=config1 --from-literal=key2=config2
   kubectl -n test run busybox --image busybox --restart=Never -- ls
   kubectl -n test delete pod busybox
   kubectl -n test delete configmap my-config
   kubectl -n kubearchive logs -l app=kubearchive-sink --tail=10000 | grep -A4 "type: dev."
   ```

### kubearchive-api-server

The Chart also includes the deployment of an api-server.
It can be deployed in debug mode when setting `.Values.apiServer.debug` to true (default to false).

The image can be hardcoded in the Chart values or `ko` can be used to dynamically build and deploy
the containers with the appropriate image.

#### Test the API Server

To test the API expose the port:

```bash
kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081
```

To check the logs run:
`kubectl logs -n kubearchive -l app=kubearchive-api-server -f`

And do a query:

```bash
curl localhost:8081/apis/apps/v1/deployments
```

#### Remote debugging API Server from the IDE

The api-server is meant to be run in a k8s cluster so this is needed also
for debugging the code.

We can use [delve](https://golangforall.com/en/post/go-docker-delve-remote-debug.html)
to run the code and be able to debug it from an IDE (VSCode or Goland).

These are the steps:

1. Deploy the chart with `ko` and `helm` in debug mode using an image with `delve`:
   ```bash
   helm install -n default [deployment name] charts/kubearchive \ 
   --set apiServer.debug=true \
   --set-string apiServer.image=$(KO_DEFAULTBASEIMAGE=gcr.io/k8s-skaffold/skaffold-debug-support/go:latest \
   ko build --disable-optimizations github.com/kubearchive/kubearchive/cmd/api) \
   --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink)
   ```
   > **_NOTE_**: To upgrade the deployment instead of running it from scratch use:
   > ```bash
   > helm upgrade -n default --reuse-values -f charts/kubearchive/values.yaml \
   > --set apiServer.debug=true \
   > --set-string apiServer.image=$(ko build --disable-optimizations \
   > github.com/kubearchive/kubearchive/cmd/api) \
   > --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
   > [deployment name] charts/kubearchive --version 0.0.1
   > ```
   > This is needed if changes are made to the code.

2. Forward the pod ports 8081 and 40000
   ```bash
   kubectl port-forward \
   $(kubectl get pods --no-headers -o custom-columns=":metadata.name" | grep api-server) \
   8081:8081 40000:40000
   ```
3. Enable breakpoints in the code using your preferred IDE
4. Connect to the process using the port 40000 in your preferred IDE:
   * [VSCode instructions](https://golangforall.com/en/post/go-docker-delve-remote-debug.html#visual-studio-code)
   * [Goland instructions](https://golangforall.com/en/post/go-docker-delve-remote-debug.html#goland-ide)
5. Query the API, e.g:
   ```bash
   curl localhost:8081/apis/apps/v1/deployments
   ```

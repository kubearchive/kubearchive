# KubeArchive Helm Chart

The kubearchive helm chart deploys the following:
* Namespace named `kubearchive`
* Service Account named `kubearchive` in the `kubearchive` namespace
* Service Account named `kubearchive-api-server` in the `kubearchive` namespace
* Deployment and Service for `kubearchive-sink` in the `kubearchive` namespace
* Deployment and Service for `kubearchive-api-server` in the `kubearchive` namespace
* Deployment, Service, Persistent Volume and Persistent Volume Claim for `kubearchive-database` in the `kubearchive` namespace
* (optionally) Namespace named `test`
* (optionally) Role named `kubearchive` in the `test` namespace
* (optionally) RoleBinding named `kubearchive` in the `test` namespace
* (optionally) Service Account named `kubearchive` in the `test` namespace
* (optionally) ApiServerSource named `api-server-source` in the `test` namespace

The settings of each resource are the same as in
[Create an ApiServerSource object](https://knative.dev/docs/eventing/sources/apiserversource/getting-started/#create-an-apiserversource-object)
tutorial of the knative docs.

## ApiServerSource Configuration

The ApiServerSource deployed by this helm chart uses the `kubearchive` service account in its namespace to watch resources
in the namespace it is deployed in. By default, it is deployed to watch for events. The `Role` and `RoleBinding`
by default give the kubearchive service account permissions to `get`, `list`, and `watch` `Events` resources in the namespace it is deployed in.
The ApiServerSource is deployed to only listen for events in namespace it was deployed in.
The resources that the ApiServerSource listens for can be
changed by running the helm chart with `kubearchive.watchNamespaces[0].role.rules[0].resources` and `kubearchive.watchNamespaces[0].apiServerSource.resources` overridden.
`kubearchive.watchNamespaces[0].role.rules[0].resources` expects that the resource type(s) list are all lowercase and plural. If one
or more of the resources in `kubearchive.watchNamespaces[0].role.rules[0].resources` is not in the kubernetes core API group, then
`kubearchive.watchNamespaces[0].role.rules[0].apiGroups` must be overridden as well to include all API groups that contain all the
resources that you are interested in. `kubearchive.watchNamespaces[0].apiServerSource.resources` is a list where each item includes the `kind` and
`apiVersion` of the resource.

## Watch other resources

More resources can be watched apart from `Events`.
Here is an example of the tweaks needed to add `Pods` and `ConfigMaps` to the watched resources:
1. In the file `values.yaml` add the resources to be watched under `apiServerSource.resources`:
    ```yaml
    # ...
    kubearchive:
      # ...
      watchNamespaces:
        - name: test
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

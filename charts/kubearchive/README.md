# KubeArchive Helm Chart

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

## ApiServerSource Configuration

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

## Watch other resources

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

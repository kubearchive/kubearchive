## Vector

To setup your development environment to use Vector observability pipeline tool
as a log collector for Loki, please install Loki without logging operator and 
run the following command: 

```bash
integrations/logging/vector/install.sh
```

This will install and configure [Vector by Datadog](https://vector.dev/docs/about/vector/) configured to collect and
send Kubernetes and Tekton logs to Loki and eventually S3.

You can check Vector logs to make sure if it is up and running:

```bash
kubectl logs -l app.kubernetes.io/name=vector -n kubearchive-vector
```

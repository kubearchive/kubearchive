## Loki

There are two options for loki log collectors, logging operator and
Vector observability pipeline.

To set up your development environment to use Loki with logging operator,
run the following command:
```bash
  /bin/bash integrations/logging/loki/install.sh --grafana
```

To set up your development environment to use Loki with Vector,
run the following command:
```bash
  /bin/bash integrations/logging/loki/install.sh --grafana --vector
```

This will install and configure:

* The [Logging Operator](https://kube-logging.dev/) configured to send logs from the cluster through `fluentd`
* [Loki](https://grafana.com/docs/loki/latest/) configured with built-in [MinIO](https://min.io/) for S3-compatible storage using a single bucket
* [Grafana](https://grafana.com/) to provide a UI to be able to explore the logs
* [Vector](https://vector.dev/) configured to send kubernetes logs directly from the cluster to loki (if --vector flag is used)

Run the log generators to create logs:
```bash
  /bin/bash test/log-generators/cronjobs/install.sh
```

### Access Loki through Grafana

1. Grafana uses the port `80` of `grafana` service. Use `kubectl` to forward the traffic.
    ```bash
    kubectl port-forward -n grafana-loki service/grafana 3000:80
    ```

1. In your browser, navigate to `http://localhost:3000`, using `admin`/`password` credentials.

1. Go to `Explore` and select `Loki`. Then write a query (e.g. `{pod="generate-log-1-29110151-kc2n9", container="generate1"}`).

1. Push `Run query` button at the top right of the screen.

### Access Loki REST API

1. Loki Gateway uses the port `80` of `loki-gateway` service to expose the REST API. Use `kubectl` to forward the traffic.
    ```bash
    kubectl port-forward -n grafana-loki service/loki-gateway 3100:80
    ```

1. Try out the REST API with `curl`. The following example is for retrieving the logs of the container `<container-name>`
   in the pod `<pod-id>`.
   ```bash
   curl -u admin:password http://localhost:3100/loki/api/v1/query_range \
    -H "X-Scope-OrgID: kubearchive" \
    --data-urlencode 'query={stream="<pod-id>-<container-name>"}"' \
    --data-urlencode 'start=2025-05-07T00:00:00Z' \
    --data-urlencode 'end=2025-05-07T23:00:00Z' \
    --data-urlencode 'limit=10' | jq '.data.result.[].values.[].[1]'
   ```
   NOTE: Even if the date range limit is disabled, it's better to have the date range shortly delimited because otherwise
   the request will time out.

# Observability Integration: Prometheus Operator

This integration aims to mimic a common pattern which is to use the Prometheus Operator
to monitor a Kubernetes cluster. In this integration there is an OpenTelemetry Collector
that receives OpenTelemetry metrics from the KubeArchive components and then exposes
them using a Prometheus Exporter. Then a `ServiceMonitor` resource instructs the Prometheus
Operator to scrap the metrics from the OpenTelemetry Collector.

**Note**: we configure KubeArchive to send traces to the OpenTelemetry collector but those
are not forwarded anywhere. This prevents KubeArchive's components to log errors for not
being able to send traces.

To visualize metrics port-forward the Grafana UI, visit http://localhost:3000 and log in with
`admin` as the user and `prom-operator` as its password:

```bash
kubectl port-forward -n prometheus-operator deployments/prometheus-grafana 3000
```

To see the metrics exposed by the OpenTelemetry Collector port-forward its port 9090:

```bash
kubectl port-forward -n kubearchive deployments/otel-collector 9090 &
head http://localhost:9090/metrics | grep db_sql_connection_open
[...]
# HELP db_sql_connection_open The number of established connections both in use and idle
# TYPE db_sql_connection_open gauge
db_sql_connection_open{db_system="postgres",job="kubearchive.api",status="idle"} 1 1743771343019
db_sql_connection_open{db_system="postgres",job="kubearchive.api",status="inuse"} 0 1743771343019
db_sql_connection_open{db_system="postgres",job="kubearchive.sink",status="idle"} 1 1743771343290
db_sql_connection_open{db_system="postgres",job="kubearchive.sink",status="inuse"} 0 1743771343290
[...]
```

To see the metrics exposed by Prometheus port-forward its 9090 and open http://localhost:9090/query:

```bash
kubectl port-forward -n prometheus-operator prometheus-prometheus-kube-prometheus-prometheus-0 9090 &
```

**Note**: Prometheus adds Kubernetes data to the OpenTelemetry metrics. The data queried from
Prometheus contain labels like:  `container=kube-rbac-proxy`, `pod=otel-collector-...`,
`service=otel-collector` so use the `job` label to filter as it retains its original value:
`job=kubearchive.sink/api/operator`.

To see logs related with the setup, run:

```bash
kubectl logs -n kubearchive deployments/otel-collector -c kube-rbac-proxy
kubectl logs -n kubearchive deployments/otel-collector -c otel-collector
kubectl logs -n prometheus-operator prometheus-prometheus-kube-prometheus-prometheus-0
```

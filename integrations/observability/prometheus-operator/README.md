# Observability Integration: Prometheus Operator

This integration aims to mimic a common pattern which is to use the Prometheus Operator
to monitor a Kubernetes cluster. In this integration there is an OpenTelemetry Collector
that receives OpenTelemetry metrics fom the KubeArchive components and then exposes
them using a Prometheus Exporter. Then a `ServiceMonitor` resource instructs the Prometheus
Operator to scrap the metrics from the OpenTelemetry Collector.

**Note**: traces are sent to the OpenTelemetry collector but they don't get forwarded
anywhere. This is done to avoid that KubeArchive's components log errors about not being
able to send traces.

To visualize metrics forward the Grafana UI, visit http://localhost:3000 and log in with
`admin` as the user and `prom-operator` as its password:

```bash
kubectl port-forward -n prometheus-operator deployments/prometheus-grafana 3000
```

To see which metrics are being exposed by the OpenTelemetry Collector port-forward
its port 9090:

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

To see which metrics are received by Prometheus port-forward its 9090 and open http://localhost:9090/query:

```bash
kubectl port-forward -n prometheus-operator prometheus-prometheus-kube-prometheus-prometheus-0 9090 &
```

**Note**: because OpenTelemetry metrics are not sent with Kubernetes data, it gets added by the Prometheus
Operator when scraping. So when you query metrics from Prometheus they contain labels like:
`container=kube-rbac-proxy`, `pod=otel-collector-...`, `service=otel-collector`, etc. However metrics
have `job=kubearchive.sink/api/operator` to tell them apart.

To see logs related with the setup run:

```bash
kubectl logs -n kubearchive deployments/otel-collector -c kube-rbac-proxy
kubectl logs -n kubearchive deployments/otel-collector -c otel-collector
kubectl logs -n prometheus-operator prometheus-prometheus-kube-prometheus-prometheus-0
```

## Elasticsearch

To set up your development environment to use Elasticsearch, run the following command:
```bash
integrations/logging/elasticsearch/install.sh
```
This will install the Elasticsearch and Logging operators into the `elastic-system` namespaces on your cluster.
It will also configure logging so that all `Pod` logs are automatically sent to Elasticsearch.

To access the Elasticsearch Kibana Dashboard from outside of the cluster, the following steps need to be taken:
1. Use `kubectl` to port forward, this will keep the terminal occupied:
    ```bash
    kubectl -n elastic-system port-forward svc/kubearchive-kb-http 5601
    ```
1. Determine the `elastic` user password for Kibana:
    ```bash
    kubectl -n elastic-system get secret kubearchive-es-elastic-user -o=jsonpath='{.data.elastic}' | base64 --decode
    ```
1. In your browser, navigate to `https://localhost:5601`, using the user name `elastic` and the password
   retrieved in the previous step to login.

## Splunk

To set up your development environment to use Splunk, run the following command:
```bash
integrations/logging/splunk/install.sh
```

This will install the Splunk and Logging operators into the `splunk-operator` namespaces on your cluster.
It will also configure logging so that all `Pod` logs are automatically sent to Splunk.

If needed, the log-generators can be used to generate more logging traffic.

### Access Splunk UI

1. Splunk uses the port `8000` of `splunk-single-standalone-service`. Use `kubectl` to forward the traffic.
    ```bash
    kubectl port-forward -n splunk-operator service/splunk-single-standalone-service 8111:8000
    ```
1. Determine the `admin` password for Splunk:
    ```bash
    echo `kubectl -n splunk-operator get secret splunk-single-standalone-secret-v1 -o jsonpath='{.data.password}' | base64 --decode`
    ```
1. In your browser, navigate to `http://localhost:8111`, using the username `admin` and the password
   retrieved in the previous step to login.

1. Go to `Search & Reporting` section and write `*` on the search box to start retrieving logs.

### Access Splunk REST API

1. Splunk uses the port `8089` of `splunk-single-standalone-service`. Use `kubectl` to forward the traffic.
    ```bash
    kubectl port-forward -n splunk-operator service/splunk-single-standalone-service 8112:8089
    ```
1. Determine the `admin` password for Splunk
    ```bash
    SPLUNK_PWD=`kubectl -n splunk-operator get secret splunk-single-standalone-secret-v1 -o jsonpath='{.data.password}' | base64 --decode`
    ```
1. Try out the REST API with `curl`. The following example is for retrieving the logs of the container `<container-name>`
   in the pod `<pod-id>` with the password `<password>`.
   ```bash
   curl -k -u admin:${SPLUNK_PWD} "https://localhost:8112/services/search/jobs/export?search=search%20%2A%20%7C%20spath%20%22kubernetes.pod_id%22%20%7C%20search%20%22kubernetes.pod_id%22%3D%22<pod-id>%22%20%7C%20spath%20%22kubernetes.container_name%22%20%7C%20search%20%22kubernetes.container_name%22%3D%22<container-name>%22%20%7C%20sort%20time%20%7C%20table%20%22message%22&output_mode=json"
   ```

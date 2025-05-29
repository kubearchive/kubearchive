## Kafka

Do not set up your KubeArchive development environment using `hack/quick-install.sh`. This script expects a clean kind
cluster with no kubearchive or knative installation. Run the following command to set up the environment"

```bash
integrations/kafka/install.sh
```

This will install all of the kubearchive dependencies, create a kafka cluster using strimzi, and install the knative
kafka extensions. Then it deploys kubearchive on top.

The knative kafka extensions conflicts with the in memory channel and the channel based broker for knative. If you
experience the installation of the knative kafka extensions hanging, try creating a new kind cluster.

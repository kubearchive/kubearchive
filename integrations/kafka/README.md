## Kafka

To set up your development environment to use Knative Eventing Kafka Brokers, run the following command

```bash
integrations/kafka/install.sh
```

This will install Strimzi, create a Kafka cluster, and install the Knative Eventing Kafka Extensions.

### Uninstall Kafka Brokers

To remove the Knative Eventing Kafka Brokers and have KubeArchive use the channel based brokers, run the following
command

```bash
integrations/kafka/uninstall.sh
```

This will revert the the KubeArchive Brokers back to MT channel based brokers. The Knative Kafka Extensions, Kafka
cluster, and Strimzi will still be installed on the cluster.

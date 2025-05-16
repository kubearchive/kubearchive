#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/..

for i in "$@"; do
    case $i in
        -k|--kafka)
            KAFKA=True
            ;;
        -h|--help)
            HELP=True
            ;;
        *)
            echo "Unknown option $i"
            HELP=True
            UNKNOWN=True
            ;;
    esac
done

KAFKA=${KAFKA:-""}
HELP=${HELP:-""}
UNKNOWN=${UNKNOWN:-""}

# Help/Usage
if [ "${HELP}" == "True" ] || [ "${UNKNOWN}" == "True" ]; then
    set +o xtrace
    echo -e "$0

    -k, --kafka    Deploy KubeArchive with Kafka Brokers instead of the default
                   MT Channel Based Brokers. This will install Strimzi and
                   the Knative Kafka Extensions.
    -h, --help     Print the usage info

    "
    if [ "${UNKNOWN}" == "True" ]; then
        exit 1
    else
        exit 0;
    fi
fi

bash integrations/database/postgresql/install.sh

# renovate: datasource=github-releases depName=cert-manager packageName=cert-manager/cert-manager
export CERT_MANAGER_VERSION=v1.17.2
# renovate: datasource=github-releases depName=knative-eventing packageName=knative/eventing
export KNATIVE_EVENTING_VERSION=v1.18.1

kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl rollout status deployment --namespace=cert-manager --timeout=30s

if [ "${KAFKA}" == "True" ]; then
    bash integrations/kafka/install.sh
else
    kubectl apply -f https://github.com/knative/eventing/releases/download/knative-${KNATIVE_EVENTING_VERSION}/eventing.yaml
    kubectl rollout status deployment --namespace=knative-eventing --timeout=50s
fi

bash hack/kubearchive-install.sh

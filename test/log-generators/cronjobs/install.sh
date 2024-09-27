#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

# Create a CronJob that run every minute, creating an Apache log file 1024 lines long.

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# Parse command line arguments
for i in "$@"
do
case $i in
    --namespace=*)
    NAMESPACE=`echo $i | sed 's/[-a-zA-Z0-9]*=//'`
    ;;
    --num-jobs=*)
    NUM_JOBS=`echo $i | sed 's/[-a-zA-Z0-9]*=//'`
    ;;
    --help)
    HELP=True
    ;;
    *)
    echo "Unknown option $i" # unknown option
    HELP=True
    UNKNOWN=True
    ;;
esac
done

HELP=${HELP:-""}
UNKNOWN=${UNKNOWN:-""}
export NAMESPACE=${NAMESPACE:-generate-logs-cronjobs}
NUM_JOBS=${NUM_JOBS:-1}

# Help and usage
if [ "${HELP}" == "True" ] || [ "${UNKNOWN}" == "True" ]; then
    echo -e "$0

    --namespace    Namespace to use to generate logs.
                   Default value is ${NAMESPACE}

    --num-jobs     Number of CronJobs to create in the namespace.
                   Default value is ${NUM_JOBS}

    "
    if [ "${UNKNOWN}" == "True" ]; then
      exit 1;
    else
      exit 0;
    fi
fi

CRONJOB=${SCRIPT_DIR}/cronjob.yaml
mv ${CRONJOB} ${CRONJOB}.orig

for i in $(seq 1 ${NUM_JOBS}); do
    sed -e "s/name: generate-log/name: generate-log-${i}/" ${CRONJOB}.orig > ${SCRIPT_DIR}/cronjob-${i}.yaml
done

kubectl create ns ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ${NAMESPACE} apply -f ${SCRIPT_DIR}

rm -f ${SCRIPT_DIR}/cronjob-*.yaml
mv ${CRONJOB}.orig ${CRONJOB}

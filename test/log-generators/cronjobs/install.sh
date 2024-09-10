#!/bin/bash

# Create a CronJob that run every minute, creating an Apache log file 1024 lines long.

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

namespace="generate-logs-cronjobs"
kubectl create ns ${namespace}
kubectl -n ${namespace} apply -f ${SCRIPT_DIR}

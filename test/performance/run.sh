#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o xtrace
set -o errexit

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

bash integrations/observability/grafana/install.sh

kubectl apply -f - <<EOF
---
apiVersion: kubearchive.org/v1
kind: KubeArchiveConfig
metadata:
  name: kubearchive
  namespace: default
spec:
  resources:
    - selector:
        apiVersion: v1
        kind: Pod
      archiveWhen: status.phase == "Succeeded"
EOF

kubectl create clusterrole view-pods --verb=list,get --resource=pods,pods/status --dry-run=client -o yaml | kubectl apply -f -
kubectl create clusterrolebinding view-pods --clusterrole view-pods --user system:serviceaccount:default:default --dry-run=client -o yaml | kubectl apply -f -
export SA_TOKEN=$(kubectl create token default --namespace default)

python3 -m venv venv
./venv/bin/python -m pip install -r ${SCRIPT_DIR}/requirements.txt

mkdir -p perf-results

START=$(date +%s)
sleep 30
kubectl port-forward -n kubearchive svc/kubearchive-sink 8082:80 &
SINK_FORWARD_PID=$!
./venv/bin/locust -f ${SCRIPT_DIR}/locustfile.py --headless --users 2 -r 2 -t 600s -H https://localhost:8081 --only-summary --processes -1 CreatePods --csv perf-results/create &> perf-results/create.txt
kill $SINK_FORWARD_PID
sleep 30
END=$(date +%s)

kubectl port-forward -n observability svc/grafana-lgtm 9090:9090 &
PROMETHEUS_FORWARD_PID=$!
go run ${SCRIPT_DIR}/metrics/main.go --start=${START} --end=${END} --prefix="./perf-results/create-"
kill $PROMETHEUS_FORWARD_PID

START=$(date +%s)
sleep 30
kubectl port-forward -n kubearchive svc/kubearchive-api-server 8081:8081 &
SINK_FORWARD_PID=$!
./venv/bin/locust -f ${SCRIPT_DIR}/locustfile.py --headless --users 2 -r 2 -t 600s -H https://localhost:8081 --only-summary --processes -1 GetPods --csv perf-results/get &> perf-results/get.txt
kill $SINK_FORWARD_PID
sleep 30
END=$(date +%s)

kubectl port-forward -n observability svc/grafana-lgtm 9090:9090 &
PROMETHEUS_FORWARD_PID=$!
go run ${SCRIPT_DIR}/metrics/main.go --start=${START} --end=${END} --prefix="./perf-results/get-"
kill $PROMETHEUS_FORWARD_PID

# We don't want to fail the report if something has changed
set +o errexit

cat <<EOF > ${GITHUB_STEP_SUMMARY:-/dev/stdout}
## Deployment Limits
\`\`\`
$(kubectl get -n kubearchive deployments.apps -l app.kubernetes.io/part-of=kubearchive -o go-template-file=${SCRIPT_DIR}/limits.gotemplate)
\`\`\`

## Create | Requests
\`\`\`
$(tail -n13 perf-results/create.txt)
\`\`\`

\`\`\`
$(kubectl exec -i -n postgresql pod/kubearchive-1 -- psql kubearchive -c "SELECT COUNT(*) FROM resource;")
\`\`\`

## Create | CPU (milliCPUs)
$(go run ${SCRIPT_DIR}/stats/main.go --file=perf-results/create-cpu.csv --type cpu)

## Create | Memory (MB)
$(go run ${SCRIPT_DIR}/stats/main.go --file=perf-results/create-memory.csv --type memory)

## Get | Requests
\`\`\`
$(tail -n13 perf-results/get.txt)
\`\`\`

## Get | CPU (milliCPUs)
$(go run ${SCRIPT_DIR}/stats/main.go --file=perf-results/get-cpu.csv --type cpu)

## Get | Memory (MB)
$(go run ${SCRIPT_DIR}/stats/main.go --file=perf-results/get-memory.csv --type memory)
EOF

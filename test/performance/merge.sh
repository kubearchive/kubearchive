#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit

OUTPUT=${GITHUB_STEP_SUMMARY:-/dev/stdout}

mkdir -p merge/
rm -f merge/workflowruns.json

go run test/performance/merge/main.go

echo -e "# Create CPU (milliCPU) \n" >> ${OUTPUT}
cat ./merge/create-cpu.csv >> ${OUTPUT}

echo -e "# Create Memory (MB) \n" >> ${OUTPUT}
cat ./merge/create-memory.csv >> ${OUTPUT}

echo -e "# Create Requests (milliseconds)\n" >> ${OUTPUT}
cat ./merge/create-req.csv >> ${OUTPUT}

echo -e "# Get CPU (milliCPU) \n" >> ${OUTPUT}
cat ./merge/get-cpu.csv >> ${OUTPUT}

echo -e "# Get Memory (MB)\n" >> ${OUTPUT}
cat ./merge/get-memory.csv >> ${OUTPUT}

echo -e "# Get Requests (milliseconds)\n" >> ${OUTPUT}
cat ./merge/get-req.csv >> ${OUTPUT}

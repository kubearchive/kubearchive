#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o xtrace
set -o errexit

RESULTS_DIR="integration-results"
FILE_PREFIX=$(date +"%d-%b-%Y_%H-%M")

if [ ! -d "${RESULTS_DIR}" ]; then
    mkdir -p ${RESULTS_DIR}
fi

go test -json -count=1 -v ./test/integration -tags=integration -timeout 60m \
    | tee ${RESULTS_DIR}/${FILE_PREFIX}_results.jsonl \
    | jq -r 'select(.Action == "output") | .Output | rtrimstr("\n")'

jq --slurp -r 'group_by(.Test) | .[] | .[] | select(.Action == "output") | .Output | rtrimstr("\n")' ${RESULTS_DIR}/${FILE_PREFIX}_results.jsonl > ${RESULTS_DIR}/${FILE_PREFIX}_formatted.txt

# Don't fail if we can't generate a summary of the action
set +o errexit

cat << EOF > ${GITHUB_STEP_SUMMARY:-/dev/stdout}
## Integration Test Logs Grouped by Test
\`\`\`
$(cat ${RESULTS_DIR}/${FILE_PREFIX}_formatted.txt)
\`\`\`

## Integration Test Logs as JSON Lines
\`\`\`
$(cat ${RESULTS_DIR}/${FILE_PREFIX}_results.jsonl)
\`\`\`
EOF

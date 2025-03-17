#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o xtrace
set -o errexit
set -o pipefail

RESULTS_DIR="integration-results"
FILE_PREFIX=$(date +"%d-%b-%Y_%H-%M")

if [ ! -d "${RESULTS_DIR}" ]; then
    mkdir -p ${RESULTS_DIR}
fi

# Don't fail immediately so we can generate a summary of the action
set +o errexit

go test -json -count=1 -v ./test/integration -tags=integration -timeout 60m \
    | tee ${RESULTS_DIR}/${FILE_PREFIX}_results.jsonl \
    | jq -r 'select(.Action == "output") | .Output | rtrimstr("\n")'

# save return code from integration test to use as return code from this script later
RET_CODE="$?"

jq --slurp -r 'group_by(.Test) | .[] | .[] | select(.Action == "output") | .Output | rtrimstr("\n")' ${RESULTS_DIR}/${FILE_PREFIX}_results.jsonl > ${RESULTS_DIR}/${FILE_PREFIX}_formatted.txt


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

# use exit code from integration test command as return code for this script
exit $RET_CODE

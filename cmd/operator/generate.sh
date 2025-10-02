#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
LOCALBIN=${SCRIPT_DIR}/bin

mkdir -p ${LOCALBIN}

go version
test -s ${LOCALBIN}/controller-gen || GOBIN=${LOCALBIN} go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17
test -s ${LOCALBIN}/setup-envtest || GOBIN=${LOCALBIN} go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.20

cd ${SCRIPT_DIR}

# Generate CRD.
echo "Generating CRD."
${LOCALBIN}/controller-gen crd paths="./..." output:dir=../../config/crds

PATCH=$(mktemp -t crd.XXXXXXXX)
cat << EOF > ${PATCH}
strategy: Webhook
webhook:
  clientConfig:
    service:
      namespace: kubearchive
      name: webhook-service
      path: /convert
  conversionReviewVersions:
  - v1
EOF
for CRD in `find ../../config/crds -name "kubearchive*.yaml"`; do
    yq -i '.metadata.annotations."cert-manager.io/inject-ca-from"="kubearchive/kubearchive-operator-certificate"' ${CRD}
    yq -i ".spec.conversion = load(\"${PATCH}\")" ${CRD}
done

rm -f ${PATCH}

# Generate role.
echo "Generating role."
${LOCALBIN}/controller-gen rbac:roleName="kubearchive-operator" \
    paths="./..." output:stdout > ../../config/templates/operator/role.yaml

echo "Generating deep copy code."
# Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
${LOCALBIN}/controller-gen object:headerFile="hack/copyright.txt" paths="./..." output:dir=api/v1

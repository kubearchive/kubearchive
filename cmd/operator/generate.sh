#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
CONTROLLER_TOOLS_VERSION="v0.14.0"
LOCALBIN=${SCRIPT_DIR}/bin

mkdir -p ${LOCALBIN}

test -s ${LOCALBIN}/controller-gen && ${LOCALBIN}/controller-gen --version | grep -q ${CONTROLLER_TOOLS_VERSION} || \
    GOBIN=${LOCALBIN} go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_VERSION}
test -s ${LOCALBIN}/setup-envtest || GOBIN=${LOCALBIN} go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

cd ${SCRIPT_DIR}

# Generate CRD.
echo "Generating CRD."
${LOCALBIN}/controller-gen crd paths="./..." output:dir=../../charts/kubearchive/crds

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
CRD="../../charts/kubearchive/crds/kubearchive.kubearchive.org_kubearchiveconfigs.yaml"
yq -i '.metadata.annotations."cert-manager.io/inject-ca-from"="kubearchive/kubearchive-operator-certificate"' ${CRD}
yq -i ".spec.conversion = load(\"${PATCH}\")" ${CRD}

rm -f ${PATCH}

# Generate role.
echo "Generating role."
${LOCALBIN}/controller-gen rbac:roleName="replaceme" \
    paths="./..." output:stdout | \
    sed -e 's/replaceme/{{ tpl .Values.operator.name . }}/' > ../../charts/kubearchive/templates/operator/role.yaml

echo "Generating deep copy code."
# Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
${LOCALBIN}/controller-gen object:headerFile="hack/copyright.txt" paths="./..." output:dir=api/v1alpha1

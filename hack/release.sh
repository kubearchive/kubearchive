#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
#
# Tools:
# * ko: https://ko.build/
# * helm: https://helm.sh/
# * gh: https://cli.github.com/
# * release-notes: https://github.com/kubernetes/release/blob/master/cmd/release-notes/README.md
#
# Externally provided variables
# export OCI_REPOSITORY="quay.io/username"
# export RELEASE_REPOSITORY="username/kubearchive"
# export GITHUB_TOKEN="token-string"  # auth for 'release-notes'
# export GH_TOKEN="token-string"  # auth for 'gh'
#
set -o errexit  # -e
set -o xtrace  # -x

# Variables
export GIT_COMMITTER_NAME="github-actions[bot]@users.noreply.github.com"
export GIT_COMMITTER_EMAIL="github-actions[bot]"
export GIT_AUTHOR_NAME=${GIT_COMMITTER_NAME}
export GIT_AUTHOR_EMAIL=${GIT_COMMITTER_EMAIL}
export KO_DOCKER_REPO="${OCI_REPOSITORY}"
export CURR_VERSION=$(cat ./VERSION)

# release-notes environment variables
export BRANCH="main"
export START_SHA=$(git rev-list -n1 ${CURR_VERSION})
export END_REV=${BRANCH}

release-notes generate \
    --required-author="" \
    --format json \
    --output ./release-notes.json \
    --repo kubearchive --org kubearchive

go run hack/get-next-version.go \
    --release-notes-file ./release-notes.json \
    --current-version ${CURR_VERSION} > ./VERSION
export NEXT_VERSION=$(cat ./VERSION)
rm ./release-notes.json

release-notes generate \
    --required-author="" \
    --output ./release-notes.md \
    --dependencies=false \
    --repo kubearchive --org kubearchive
echo -e "# Release notes for ${NEXT_VERSION}\n" >> ${GITHUB_STEP_SUMMARY:-/dev/stdout}
cat ./release-notes.md >> ${GITHUB_STEP_SUMMARY:-/dev/stdout}

git add VERSION
git commit -s -m "Release ${NEXT_VERSION}"

# Build and push
bash cmd/operator/generate.sh
ko build github.com/kubearchive/kubearchive/cmd/sink --base-import-paths --tags=${NEXT_VERSION}
ko build github.com/kubearchive/kubearchive/cmd/api --base-import-paths --tags=${NEXT_VERSION}
ko build github.com/kubearchive/kubearchive/cmd/operator/ --base-import-paths --tags=${NEXT_VERSION}

helm package charts/kubearchive --app-version ${NEXT_VERSION} --version ${NEXT_VERSION}
helm push kubearchive-helm-${NEXT_VERSION}.tgz oci://${OCI_REPOSITORY}

git tag -a "${NEXT_VERSION}" -m "Release ${NEXT_VERSION}"
git push
git push --tags

gh release create "${NEXT_VERSION}" \
    --notes-file ./release-notes.md \
    --title "Release ${NEXT_VERSION}" \
    --repo ${RELEASE_REPOSITORY}
rm ./release-notes.md

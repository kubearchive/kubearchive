# KubeArchive Installer

To create the installer standalone use:

```bash
bash integrations/database/postgresql/install.sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml
kubectl rollout status deployment --namespace=cert-manager --timeout=30s

export KO_DOCKER_REPO="kind.local"
bash cmd/installer/generate.sh
kubectl kustomize config/installer/standalone | ko apply --tags latest-build -f - --base-import-paths
ko build github.com/kubearchive/kubearchive/cmd/vacuum --tags latest-build --base-import-paths  # this is required to pass integration tests
kubectl apply -f - <<EOF
---
apiVersion: kubearchive.org/v1
kind: KubeArchiveInstallation
metadata:
  name: kubearchive
spec:
  version: v1.18.1
EOF

kubectl get pods -n kubearchive
```

To create the installer using Operator Lifecycle Manager (OLM) do:

```bash
operator-sdk olm install

bash integrations/database/postgresql/install.sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml
kubectl rollout status deployment --namespace=cert-manager --timeout=30s

export KO_DOCKER_REPO="quay.io/<your-org>"
ko build github.com/kubearchive/kubearchive/cmd/installer/ --tags v0.0.1 --base-import-paths
yq -i ".images[0].newName = \"${KO_DOCKER_REPO}/installer\"" config/installer/bundle/kustomization.yaml
yq -i ".images[0].newTag = \"v0.0.1\"" config/installer/bundle/kustomization.yaml
kubectl kustomize config/installer/bundle | operator-sdk generate bundle --package kubearchive-installer --version 0.0.1
podman build -t ${KO_DOCKER_REPO}/installer-bundle:v0.0.1 -f bundle.Dockerfile .
podman push ${KO_DOCKER_REPO}/installer-bundle:v0.0.1
operator-sdk run bundle ${KO_DOCKER_REPO}/installer-bundle:v0.0.1
kubectl apply -f - <<EOF
---
apiVersion: kubearchive.org/v1
kind: KubeArchiveInstallation
metadata:
  name: kubearchive
spec:
  version: v1.17.2
EOF
kubectl get pods -n kubearchive
```

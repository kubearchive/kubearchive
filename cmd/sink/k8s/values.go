// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import "os"

const globalKeyEnvVar = "KUBEARCHIVE_NAMESPACE"

var KubeArchiveNamespace string

func init() {
	KubeArchiveNamespace = os.Getenv(globalKeyEnvVar)
}

func SetKubeArchiveNamespaceTest() {
	KubeArchiveNamespace = "kubearchive"
}

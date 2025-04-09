// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"os"
)

const (
	SinkFiltersConfigMapName   = "sink-filters"
	SinkFiltersGlobalNamespace = "___global___"
	nsEnvVar                   = "KUBEARCHIVE_NAMESPACE"
)

var (
	KubeArchiveNamespace string // gets set in init() and should be treated as const
)

func init() {
	KubeArchiveNamespace = os.Getenv(nsEnvVar)
	if KubeArchiveNamespace == "" {
		KubeArchiveNamespace = "kubearchive" // Not set for testing!
	}
}

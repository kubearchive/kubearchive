// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"os"
)

const (
	KubeArchiveConfigResourceName = "kubearchive"
	KubeArchiveNamespaceEnvVar    = "KUBEARCHIVE_NAMESPACE"
	SinkFilterResourceName        = "sink-filters"
	SinkFilterGlobalNamespace     = "___global___"
)

var (
	// These get set in init() and should be treated as const
	KubeArchiveNamespace  string
	KubeArchiveBrokerName string
	KubeArchiveSinkName   string
)

func init() {
	KubeArchiveNamespace = os.Getenv(KubeArchiveNamespaceEnvVar)
	if KubeArchiveNamespace == "" {
		KubeArchiveNamespace = "kubearchive" // Not set for testing!
	}
	KubeArchiveBrokerName = KubeArchiveNamespace + "-broker"
	KubeArchiveSinkName = KubeArchiveNamespace + "-sink"
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"os"
)

const (
	KubeArchiveConfigResourceName          = "kubearchive"
	KubeArchiveNamespaceEnvVar             = "KUBEARCHIVE_NAMESPACE"
	SinkFilterResourceName                 = "sink-filters"
	KubeArchiveSinkName                    = "kubearchive-sink"
	KubeArchiveOperatorWebhooksServiceName = "kubearchive-operator-webhooks"
	KubeArchiveSinkFilterName              = "kubearchive-sinkfilter"
	KubeArchiveOperatorName                = "kubearchive-operator"
	SinkFilterGlobalNamespace              = "___global___"
	ClusterVacuumAllNamespaces             = "___all-namespaces___"
	KubeArchiveVacuumName                  = "kubearchive-vacuum"
	KubeArchiveClusterVacuumName           = "kubearchive-cluster-vacuum"
)

var (
	// These get set in init() and should be treated as const
	KubeArchiveNamespace string
)

func init() {
	KubeArchiveNamespace = os.Getenv(KubeArchiveNamespaceEnvVar)
	if KubeArchiveNamespace == "" {
		KubeArchiveNamespace = "kubearchive" // Not set for testing!
	}
}

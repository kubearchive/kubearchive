// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"os"
)

const (
	ClusterKubeArchiveConfigClusterRoleBindingName = "clusterkubearchiveconfig-read"
	KubeArchiveConfigResourceName                  = "kubearchive"
	KubeArchiveNamespaceEnvVar                     = "KUBEARCHIVE_NAMESPACE"
	SinkFilterResourceName                         = "sink-filters"
	KubeArchiveSinkName                            = "kubearchive-sink"
	KubeArchiveApiServerSourceName                 = "kubearchive-a13e"
	SinkFilterGlobalNamespace                      = "___global___"
	ClusterVacuumAllNamespaces                     = "___all-namespaces___"
	KubeArchiveBrokerName                          = "kubearchive-broker"
	KubeArchiveVacuumName                          = "kubearchive-vacuum"
	KubeArchiveVacuumBroker                        = "kubearchive-broker"
	KubeArchiveClusterVacuumName                   = "kubearchive-cluster-vacuum"
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

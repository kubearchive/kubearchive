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
	SinkFilterGlobalNamespace                      = "___global___"
	ClusterVacuumAllNamespaces                     = "___all-namespaces___"
)

var (
	// These get set in init() and should be treated as const
	KubeArchiveNamespace           string
	KubeArchiveBrokerName          string
	KubeArchiveSinkName            string
	KubeArchiveVacuumName          string
	KubeArchiveVacuumBroker        string
	KubeArchiveClusterVacuumName   string
	KubeArchiveApiServerSourceName string
)

func init() {
	KubeArchiveNamespace = os.Getenv(KubeArchiveNamespaceEnvVar)
	if KubeArchiveNamespace == "" {
		KubeArchiveNamespace = "kubearchive" // Not set for testing!
	}
	KubeArchiveBrokerName = KubeArchiveNamespace + "-broker"
	KubeArchiveSinkName = KubeArchiveNamespace + "-sink"
	KubeArchiveVacuumName = KubeArchiveNamespace + "-vacuum"
	KubeArchiveVacuumBroker = KubeArchiveVacuumName + "-broker"
	KubeArchiveClusterVacuumName = KubeArchiveNamespace + "-cluster-vacuum"
	KubeArchiveApiServerSourceName = KubeArchiveNamespace + "-a13e"
}

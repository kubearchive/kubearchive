// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"os"

	"github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"gopkg.in/yaml.v3"
)

// KACResourceSliceFromFile attempts to read []KubeArchiveConfigResource from file located at path. It assumes that the
// the []KubeArchiveConfigResource is in YAML format.
func KACResourceSliceFromFile(path string) ([]v1alpha1.KubeArchiveConfigResource, error) {
	resources := []v1alpha1.KubeArchiveConfigResource{}
	resourcesBytes, err := os.ReadFile(path)
	if err != nil {
		return resources, err
	}
	err = yaml.Unmarshal(resourcesBytes, &resources)
	return resources, err
}

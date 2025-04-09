// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

type KubeArchiveConfigResource struct {
	Selector        sourcesv1.APIVersionKindSelector `json:"selector,omitempty" yaml:"selector,omitempty"`
	ArchiveWhen     string                           `json:"archiveWhen,omitempty" yaml:"archiveWhen,omitempty"`
	DeleteWhen      string                           `json:"deleteWhen,omitempty" yaml:"deleteWhen,omitempty"`
	ArchiveOnDelete string                           `json:"archiveOnDelete,omitempty" yaml:"archiveOnDelete,omitempty"`
}

// KubeArchiveConfigSpec defines the desired state of KubeArchiveConfig
type KubeArchiveConfigSpec struct {
	Resources []KubeArchiveConfigResource `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// KubeArchiveConfigStatus defines the observed state of KubeArchiveConfig
type KubeArchiveConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=kac;kacs
//+kubebuilder:subresource:status

// KubeArchiveConfig is the Schema for the kubearchiveconfigs API
type KubeArchiveConfig struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   KubeArchiveConfigSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status KubeArchiveConfigStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubeArchiveConfigList contains a list of KubeArchiveConfig
type KubeArchiveConfigList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items           []KubeArchiveConfig `json:"items" yaml:"items"`
}

func LoadKubeArchiveConfigFromFile(path string) ([]KubeArchiveConfigResource, error) {
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		return []KubeArchiveConfigResource{}, err
	}
	return LoadKubeArchiveConfigFromString(string(yamlBytes))
}

func LoadKubeArchiveConfigFromString(yamlString string) ([]KubeArchiveConfigResource, error) {
	// Go through the hoops of unmarshalling to JSON so that fields are not lost. Not
	// all structs have yaml tags on them, which can cause issues when YAML is used directly.
	var data []interface{}

	// Unmarshal YAML string to generic array
	err := yaml.Unmarshal([]byte(yamlString), &data)
	if err != nil {
		return nil, err
	}
	// Marshal to JSON bytes
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	// Unmarshall using json package to better preserve data.
	resources := []KubeArchiveConfigResource{}
	err = json.Unmarshal(jsonBytes, &resources)
	if err != nil {
		return nil, err
	}
	return resources, err
}

func init() {
	SchemeBuilder.Register(&KubeArchiveConfig{}, &KubeArchiveConfigList{})
}

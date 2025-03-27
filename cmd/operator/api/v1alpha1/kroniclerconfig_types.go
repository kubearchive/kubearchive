// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"os"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

type KroniclerConfigResource struct {
	Selector        sourcesv1.APIVersionKindSelector `json:"selector,omitempty"`
	ArchiveWhen     string                           `json:"archiveWhen,omitempty"`
	DeleteWhen      string                           `json:"deleteWhen,omitempty"`
	ArchiveOnDelete string                           `json:"archiveOnDelete,omitempty"`
}

// KroniclerConfigSpec defines the desired state of KroniclerConfig
type KroniclerConfigSpec struct {
	Resources []KroniclerConfigResource `json:"resources,omitempty"`
}

// KroniclerConfigStatus defines the observed state of KroniclerConfig
type KroniclerConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=kron;krons
//+kubebuilder:subresource:status

// KroniclerConfig is the Schema for the kroniclerconfigs API
type KroniclerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KroniclerConfigSpec   `json:"spec,omitempty"`
	Status KroniclerConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KroniclerConfigList contains a list of KroniclerConfig
type KroniclerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KroniclerConfig `json:"items"`
}

func LoadFromFile(path string) ([]KroniclerConfigResource, error) {
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		return []KroniclerConfigResource{}, err
	}
	return LoadFromString(string(yamlBytes))
}

func LoadFromString(yamlString string) ([]KroniclerConfigResource, error) {
	resources := []KroniclerConfigResource{}
	err := yaml.Unmarshal([]byte(yamlString), &resources)
	return resources, err
}

func init() {
	SchemeBuilder.Register(&KroniclerConfig{}, &KroniclerConfigList{})
}

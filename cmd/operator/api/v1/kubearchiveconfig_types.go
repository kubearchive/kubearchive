// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +kubebuilder:object:generate=true
type APIVersionKind struct {
	Kind       string `json:"kind" yaml:"kind"`
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
}

var KubeArchiveConfigGVR = schema.GroupVersionResource{Group: "kubearchive.org", Version: "v1", Resource: "kubearchiveconfigs"}

type KubeArchiveConfigResource struct {
	Selector        APIVersionKind `json:"selector,omitempty" yaml:"selector,omitempty"`
	ArchiveWhen     string         `json:"archiveWhen,omitempty" yaml:"archiveWhen,omitempty"`
	DeleteWhen      string         `json:"deleteWhen,omitempty" yaml:"deleteWhen,omitempty"`
	ArchiveOnDelete string         `json:"archiveOnDelete,omitempty" yaml:"archiveOnDelete,omitempty"`
}

// KubeArchiveConfigSpec defines the desired state of KubeArchiveConfig
type KubeArchiveConfigSpec struct {
	Resources []KubeArchiveConfigResource `json:"resources" yaml:"resources"`
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

func ConvertUnstructuredToKubeArchiveConfig(object *unstructured.Unstructured) (*KubeArchiveConfig, error) {
	bytes, err := object.MarshalJSON()
	if err != nil {
		return nil, err
	}

	kac := &KubeArchiveConfig{}
	if err := json.Unmarshal(bytes, kac); err != nil {
		return nil, err
	}
	return kac, nil
}

func init() {
	SchemeBuilder.Register(&KubeArchiveConfig{}, &KubeArchiveConfigList{})
}

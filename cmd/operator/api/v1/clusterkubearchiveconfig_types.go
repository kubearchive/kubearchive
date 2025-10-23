// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ClusterKubeArchiveConfigGVR = schema.GroupVersionResource{Group: "kubearchive.org", Version: "v1", Resource: "clusterkubearchiveconfigs"}

// +kubebuilder:object:generate=true
type ClusterKeepLastRule struct {
	Name  string `json:"name" yaml:"name"`
	Count int    `json:"count" yaml:"count"`
	When  string `json:"when" yaml:"when"`
	// +kubebuilder:default="metadata.creationTimestamp"
	SortBy string `json:"sortBy,omitempty" yaml:"sortBy,omitempty"`
}

type ClusterKubeArchiveConfigResource struct {
	Selector        APIVersionKind        `json:"selector,omitempty" yaml:"selector,omitempty"`
	ArchiveWhen     string                `json:"archiveWhen,omitempty" yaml:"archiveWhen,omitempty"`
	DeleteWhen      string                `json:"deleteWhen,omitempty" yaml:"deleteWhen,omitempty"`
	ArchiveOnDelete string                `json:"archiveOnDelete,omitempty" yaml:"archiveOnDelete,omitempty"`
	KeepLastWhen    []ClusterKeepLastRule `json:"keepLastWhen,omitempty" yaml:"keepLastWhen,omitempty"`
}

// ClusterKubeArchiveConfigSpec defines the desired state of ClusterKubeArchiveConfig
type ClusterKubeArchiveConfigSpec struct {
	Resources []ClusterKubeArchiveConfigResource `json:"resources" yaml:"resources"`
}

// ClusterKubeArchiveConfigStatus defines the observed state of ClusterKubeArchiveConfig
type ClusterKubeArchiveConfigStatus KubeArchiveConfigStatus

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster,shortName=ckac;ckacs
//+kubebuilder:subresource:status

// ClusterKubeArchiveConfig is the Schema for the clusterkubearchiveconfigs API
type ClusterKubeArchiveConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterKubeArchiveConfigSpec   `json:"spec,omitempty"`
	Status ClusterKubeArchiveConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterKubeArchiveConfigList contains a list of ClusterKubeArchiveConfig
type ClusterKubeArchiveConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterKubeArchiveConfig `json:"items"`
}

func ConvertUnstructuredToClusterKubeArchiveConfig(object *unstructured.Unstructured) (*ClusterKubeArchiveConfig, error) {
	bytes, err := object.MarshalJSON()
	if err != nil {
		return nil, err
	}

	ckac := &ClusterKubeArchiveConfig{}

	if err := json.Unmarshal(bytes, ckac); err != nil {
		return nil, err
	}
	return ckac, nil
}

func init() {
	SchemeBuilder.Register(&ClusterKubeArchiveConfig{}, &ClusterKubeArchiveConfigList{})
}

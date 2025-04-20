// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ClusterVacuumConfigGVR = schema.GroupVersionResource{Group: "kubearchive.kubearchive.org", Version: "v1alpha1", Resource: "clustervacuumconfigs"}

type ClusterVacuumConfigNamespaceSpec struct {
	Name                      string `json:"name" yaml:"name"`
	NamespaceVacuumConfigSpec `json:",inline"`
}

// ClusterVacuumConfigSpec defines the desired state of ClusterVacuumConfig resource
type ClusterVacuumConfigSpec struct {
	Namespaces []ClusterVacuumConfigNamespaceSpec `json:"namespaces" yaml:"namespaces"`
}

// ClusterVacuumConfigStatus defines the observed state of ClusterVacuumConfig resource
type ClusterVacuumConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=cvc;cvcs
//+kubebuilder:subresource:status

// ClusterVacuumConfig is the Schema for the clustervacuumconfigs API
type ClusterVacuumConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterVacuumConfigSpec   `json:"spec,omitempty"`
	Status ClusterVacuumConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterVacuumConfigList contains a list of ClusterVacuumConfig resources
type ClusterVacuumConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterVacuumConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterVacuumConfig{}, &ClusterVacuumConfigList{})
}

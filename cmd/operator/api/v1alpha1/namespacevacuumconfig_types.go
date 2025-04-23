// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

var NamespaceVacuumConfigGVR = schema.GroupVersionResource{Group: "kubearchive.kubearchive.org", Version: "v1alpha1", Resource: "namespacevacuumconfigss"}

// VacuumListSpec defines the desired state of VacuumList resource
type NamespaceVacuumConfigSpec struct {
	Resources []sourcesv1.APIVersionKind `json:"resources" yaml:"resources"`
}

// NamespaceVacuumConfigStatus defines the observed state of NamespaceVacuumConfig resource
type NamespaceVacuumConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=nvc;nvcs
//+kubebuilder:subresource:status

// NamespaceVacuumConfig is the Schema for the namespacevacuumconfigs API
type NamespaceVacuumConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceVacuumConfigSpec   `json:"spec,omitempty"`
	Status NamespaceVacuumConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NamespaceVacuumConfigList contains a list of NamespaceVacuumConfig resources
type NamespaceVacuumConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NamespaceVacuumConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceVacuumConfig{}, &NamespaceVacuumConfigList{})
}

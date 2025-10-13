// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var NamespaceVacuumConfigGVR = schema.GroupVersionResource{Group: "kubearchive.org", Version: "v1", Resource: "namespacevacuumconfigs"}

// VacuumListSpec defines the desired state of VacuumList resource
type NamespaceVacuumConfigSpec struct {
	Resources []APIVersionKind `json:"resources,omitempty" yaml:"resources"`
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

func ConvertObjectToNamespaceVacuumConfig(object runtime.Object) (*NamespaceVacuumConfig, error) {
	unstructuredData, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{Object: unstructuredData}

	return ConvertUnstructuredToNamespaceVacuumConfig(obj)
}

func ConvertUnstructuredToNamespaceVacuumConfig(object *unstructured.Unstructured) (*NamespaceVacuumConfig, error) {
	bytes, err := object.MarshalJSON()
	if err != nil {
		return nil, err
	}

	config := &NamespaceVacuumConfig{}

	if err := json.Unmarshal(bytes, config); err != nil {
		return nil, err
	}
	return config, nil
}

func init() {
	SchemeBuilder.Register(&NamespaceVacuumConfig{}, &NamespaceVacuumConfigList{})
}

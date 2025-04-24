// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ClusterVacuumConfigGVR = schema.GroupVersionResource{Group: "kubearchive.kubearchive.org", Version: "v1alpha1", Resource: "clustervacuumconfigs"}

type ClusterVacuumConfigNamespaceSpec struct {
	NamespaceVacuumConfigSpec `json:",inline,omitempty"`
}

// ClusterVacuumConfigSpec defines the desired state of ClusterVacuumConfig resource
type ClusterVacuumConfigSpec struct {
	Namespaces map[string]ClusterVacuumConfigNamespaceSpec `json:"namespaces,omitempty" yaml:"namespaces"`
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

func ConvertObjectToClusterVacuumConfig(object runtime.Object) (*ClusterVacuumConfig, error) {
	unstructuredData, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{Object: unstructuredData}

	return ConvertUnstructuredToClusterVacuumConfig(obj)
}

func ConvertUnstructuredToClusterVacuumConfig(object *unstructured.Unstructured) (*ClusterVacuumConfig, error) {
	bytes, err := object.MarshalJSON()
	if err != nil {
		return nil, err
	}

	config := &ClusterVacuumConfig{}

	if err := json.Unmarshal(bytes, config); err != nil {
		return nil, err
	}
	return config, nil
}

func init() {
	SchemeBuilder.Register(&ClusterVacuumConfig{}, &ClusterVacuumConfigList{})
}

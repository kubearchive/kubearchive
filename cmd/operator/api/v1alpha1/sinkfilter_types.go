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

var SinkFilterGVR = schema.GroupVersionResource{Group: "kubearchive.org", Version: "v1alpha1", Resource: "sinkfilters"}

// SinkFilterSpec defines the desired state of SinkFilter resource
type SinkFilterSpec struct {
	Namespaces map[string][]KubeArchiveConfigResource `json:"namespaces" yaml:"namespaces"`
}

// SinkFilterStatus defines the observed state of SinkFilter resource
type SinkFilterStatus KubeArchiveConfigStatus

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=sf;sfs
//+kubebuilder:subresource:status

// SinkFilter is the Schema for the sinkfilters API
type SinkFilter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SinkFilterSpec   `json:"spec,omitempty"`
	Status SinkFilterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SinkFilterList contains a list of SinkFilter resources
type SinkFilterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SinkFilter `json:"items"`
}

func ConvertObjectToSinkFilter(object runtime.Object) (*SinkFilter, error) {
	unstructuredData, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{Object: unstructuredData}

	return ConvertUnstructuredToSinkFilter(obj)
}

func ConvertUnstructuredToSinkFilter(object *unstructured.Unstructured) (*SinkFilter, error) {
	bytes, err := object.MarshalJSON()
	if err != nil {
		return nil, err
	}

	sinkFilter := &SinkFilter{}

	if err := json.Unmarshal(bytes, sinkFilter); err != nil {
		return nil, err
	}
	return sinkFilter, nil
}

func init() {
	SchemeBuilder.Register(&SinkFilter{}, &SinkFilterList{})
}

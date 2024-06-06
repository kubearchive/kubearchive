// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeArchiveConfigSpec defines the desired state of KubeArchiveConfig
type KubeArchiveConfigSpec struct {
	Filter string `json:"filter,omitempty"`
}

// KubeArchiveConfigStatus defines the observed state of KubeArchiveConfig
type KubeArchiveConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=kac;kacs
//+kubebuilder:subresource:status

// KubeArchiveConfig is the Schema for the kubearchiveconfigs API
type KubeArchiveConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeArchiveConfigSpec   `json:"spec,omitempty"`
	Status KubeArchiveConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubeArchiveConfigList contains a list of KubeArchiveConfig
type KubeArchiveConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeArchiveConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeArchiveConfig{}, &KubeArchiveConfigList{})
}

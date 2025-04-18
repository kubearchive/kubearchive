// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterKubeArchiveConfigSpec defines the desired state of ClusterKubeArchiveConfig
type ClusterKubeArchiveConfigSpec KubeArchiveConfigSpec

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

func init() {
	SchemeBuilder.Register(&ClusterKubeArchiveConfig{}, &ClusterKubeArchiveConfigList{})
}

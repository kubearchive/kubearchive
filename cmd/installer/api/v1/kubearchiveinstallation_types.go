// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KubeArchiveInstallationSpec defines the desired state of KubeArchiveInstallation.
type KubeArchiveInstallationSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Version is the version of KubeArchive to install. See https://github.com/kubearchive/kubearchive/releases
	// for a list of available versions
	Version string `json:"version"`
}

type Manifest struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// KubeArchiveInstallationStatus defines the observed state of KubeArchiveInstallation.
type KubeArchiveInstallationStatus struct {
	Manifests []Manifest `json:"manifests"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KubeArchiveInstallation is the Schema for the kubearchiveinstallations API.
type KubeArchiveInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeArchiveInstallationSpec   `json:"spec"`
	Status KubeArchiveInstallationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeArchiveInstallationList contains a list of KubeArchiveInstallation.
type KubeArchiveInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeArchiveInstallation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeArchiveInstallation{}, &KubeArchiveInstallationList{})
}

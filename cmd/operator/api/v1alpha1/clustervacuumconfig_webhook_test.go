// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

func TestClusterVacuumConfigCustomDefaulter(t *testing.T) {
	defaulter := ClusterVacuumConfigCustomDefaulter{}
	cvc := &ClusterVacuumConfig{}
	err := defaulter.Default(context.Background(), cvc)
	assert.NoError(t, err)
	assert.Equal(t, ClusterVacuumConfigSpec{Namespaces: nil}, cvc.Spec)
}

func TestClusterVacuumConfigValidateName(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		validated bool
	}{
		{
			name:      "valid-name",
			namespace: constants.KubeArchiveNamespace,
			validated: true,
		},
		{
			name:      "invalid-name",
			namespace: "other-namespace",
			validated: false,
		},
	}
	validator := ClusterVacuumConfigCustomValidator{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			cvc := &ClusterVacuumConfig{ObjectMeta: metav1.ObjectMeta{Namespace: test.namespace, Name: "cvac"}}
			warns, err := validator.ValidateCreate(context.Background(), cvc)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource namespace %s", test.namespace)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &ClusterVacuumConfig{}, cvc)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource namespace %s", test.namespace)
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), cvc)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

func TestClusterVacuumConfigValidateResources(t *testing.T) {
	tests := []struct {
		name      string
		spec      ClusterVacuumConfigSpec
		validated bool
	}{
		{
			name:      "No namespaces",
			spec:      ClusterVacuumConfigSpec{},
			validated: true,
		},
		{
			name:      "Empty namespaces",
			spec:      ClusterVacuumConfigSpec{Namespaces: map[string]ClusterVacuumConfigNamespaceSpec{}},
			validated: true,
		},
		{
			name: "One namespace",
			spec: ClusterVacuumConfigSpec{
				Namespaces: map[string]ClusterVacuumConfigNamespaceSpec{
					"namespace-1": {},
				},
			},
			validated: true,
		},
		{
			name: "Muliple namespaces",
			spec: ClusterVacuumConfigSpec{
				Namespaces: map[string]ClusterVacuumConfigNamespaceSpec{
					"namespace-1": {},
					"namespace-2": {},
				},
			},
			validated: true,
		},
		{
			name: "With one resource",
			spec: ClusterVacuumConfigSpec{
				Namespaces: map[string]ClusterVacuumConfigNamespaceSpec{
					"namespace-1": {},
					"namespace-2": {
						NamespaceVacuumConfigSpec: NamespaceVacuumConfigSpec{
							Resources: []sourcesv1.APIVersionKind{
								{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
							},
						},
					},
				},
			},
			validated: true,
		},
		{
			name: "With multiple resources",
			spec: ClusterVacuumConfigSpec{
				Namespaces: map[string]ClusterVacuumConfigNamespaceSpec{
					"namespace-1": {},
					"namespace-2": {
						NamespaceVacuumConfigSpec: NamespaceVacuumConfigSpec{
							Resources: []sourcesv1.APIVersionKind{
								{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
								{APIVersion: "tekton.dev/v1", Kind: "TaskRun"},
							},
						},
					},
				},
			},
			validated: true,
		},
		{
			name: "Non-configured resource",
			spec: ClusterVacuumConfigSpec{
				Namespaces: map[string]ClusterVacuumConfigNamespaceSpec{
					"namespace-1": {},
					"namespace-2": {
						NamespaceVacuumConfigSpec: NamespaceVacuumConfigSpec{
							Resources: []sourcesv1.APIVersionKind{
								{APIVersion: "v1", Kind: "Event"},
							},
						},
					},
				},
				
			},
			validated: false,
		},
	}

	ckac := &ClusterKubeArchiveConfig{
		ObjectMeta: metav1.ObjectMeta{Name: constants.KubeArchiveConfigResourceName},
		Spec: ClusterKubeArchiveConfigSpec{
			Resources: []KubeArchiveConfigResource{{
				Selector: sourcesv1.APIVersionKindSelector{
					APIVersion: "tekton.dev/v1",
					Kind:       "PipelineRun",
				},
			}},
		},
	}
	kac1 := &KubeArchiveConfig{
		ObjectMeta: metav1.ObjectMeta{Name: constants.KubeArchiveConfigResourceName, Namespace: "namespace-1"},
		Spec: KubeArchiveConfigSpec{
			Resources: []KubeArchiveConfigResource{{
				Selector: sourcesv1.APIVersionKindSelector{
					APIVersion: "tekton.dev/v1",
					Kind:       "TaskRun",
				},
			}},
		},
	}
	kac2 := &KubeArchiveConfig{
		ObjectMeta: metav1.ObjectMeta{Name: constants.KubeArchiveConfigResourceName, Namespace: "namespace-2"},
		Spec: KubeArchiveConfigSpec{
			Resources: []KubeArchiveConfigResource{{
				Selector: sourcesv1.APIVersionKindSelector{
					APIVersion: "tekton.dev/v1",
					Kind:       "TaskRun",
				},
			}},
		},
	}
	scheme := runtime.NewScheme()
	AddToScheme(scheme) //nolint:errcheck

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			dynaClient = fake.NewSimpleDynamicClient(scheme, ckac, kac1, kac2)
			validator := ClusterVacuumConfigCustomValidator{}

			// Create resource
			nvc := &ClusterVacuumConfig{ObjectMeta: metav1.ObjectMeta{Name: "nvc", Namespace: constants.KubeArchiveNamespace}, Spec: test.spec}
			warns, err := validator.ValidateCreate(context.Background(), nvc)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource in test %s", test.name)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &ClusterVacuumConfig{}, nvc)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.name)
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), nvc)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

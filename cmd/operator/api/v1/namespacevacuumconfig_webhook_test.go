// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestNamespaceVacuumConfigCustomDefaulter(t *testing.T) {
	defaulter := NamespaceVacuumConfigCustomDefaulter{}
	nvc := &NamespaceVacuumConfig{}
	err := defaulter.Default(context.Background(), nvc)
	assert.NoError(t, err)
	assert.Equal(t, NamespaceVacuumConfigSpec{Resources: nil}, nvc.Spec)
}

func TestNamespaceVacuumConfigValidateResources(t *testing.T) {
	tests := []struct {
		name      string
		spec      NamespaceVacuumConfigSpec
		validated bool
	}{
		{
			name:      "No resources",
			spec:      NamespaceVacuumConfigSpec{},
			validated: true,
		},
		{
			name:      "Empty resources",
			spec:      NamespaceVacuumConfigSpec{Resources: []APIVersionKind{}},
			validated: true,
		},
		{
			name: "One resource",
			spec: NamespaceVacuumConfigSpec{
				Resources: []APIVersionKind{
					{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
				}},
			validated: true,
		},
		{
			name: "Muliple resources",
			spec: NamespaceVacuumConfigSpec{
				Resources: []APIVersionKind{
					{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
					{APIVersion: "tekton.dev/v1", Kind: "TaskRun"},
				}},
			validated: true,
		},
		{
			name: "Non configured resource",
			spec: NamespaceVacuumConfigSpec{
				Resources: []APIVersionKind{
					{APIVersion: "v1", Kind: "Event"},
				}},
			validated: false,
		},
	}

	ckac := &ClusterKubeArchiveConfig{
		ObjectMeta: metav1.ObjectMeta{Name: constants.KubeArchiveConfigResourceName},
		Spec: ClusterKubeArchiveConfigSpec{
			Resources: []KubeArchiveConfigResource{{
				Selector: APIVersionKind{
					APIVersion: "tekton.dev/v1",
					Kind:       "TaskRun",
				},
			}},
		},
	}
	kac := &KubeArchiveConfig{
		ObjectMeta: metav1.ObjectMeta{Name: constants.KubeArchiveConfigResourceName, Namespace: "default"},
		Spec: KubeArchiveConfigSpec{
			Resources: []KubeArchiveConfigResource{{
				Selector: APIVersionKind{
					APIVersion: "tekton.dev/v1",
					Kind:       "PipelineRun",
				},
			}},
		},
	}
	scheme := runtime.NewScheme()
	AddToScheme(scheme) //nolint:errcheck
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			dynaClient = fake.NewSimpleDynamicClient(scheme, ckac, kac)
			validator := NamespaceVacuumConfigCustomValidator{}

			// Create resource
			nvc := &NamespaceVacuumConfig{ObjectMeta: metav1.ObjectMeta{Name: "nvc", Namespace: "default"}, Spec: test.spec}
			warns, err := validator.ValidateCreate(context.Background(), nvc)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource in test %s", test.name)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &NamespaceVacuumConfig{}, nvc)
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

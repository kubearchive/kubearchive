// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"testing"

	"github.com/google/cel-go/cel"
	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createTestSinkFilter() *kubearchivev1.SinkFilter {
	return &kubearchivev1.SinkFilter{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubearchive.org/v1",
			Kind:       "SinkFilter",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SinkFilterResourceName,
			Namespace: constants.KubeArchiveNamespace,
		},
		Spec: kubearchivev1.SinkFilterSpec{
			Cluster: []kubearchivev1.ClusterKubeArchiveConfigResource{
				{
					Selector: kubearchivev1.APIVersionKind{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ArchiveWhen:     "metadata.labels.archive == 'true'",
					DeleteWhen:      "",
					ArchiveOnDelete: "",
				},
			},
			Namespaces: map[string][]kubearchivev1.KubeArchiveConfigResource{
				"test-namespace": {
					{
						Selector: kubearchivev1.APIVersionKind{
							Kind:       "Pod",
							APIVersion: "v1",
						},
						ArchiveWhen:     "true",
						DeleteWhen:      "false",
						ArchiveOnDelete: "metadata.name == 'test'",
					},
				},
			},
		},
	}
}

func TestCelExpressions(t *testing.T) {
	tests := []struct {
		name     string
		celExpr  CelExpressions
		expected CelExpressions
	}{
		{
			name: "Empty CelExpressions",
			celExpr: CelExpressions{
				ArchiveWhen:     nil,
				DeleteWhen:      nil,
				ArchiveOnDelete: nil,
			},
			expected: CelExpressions{
				ArchiveWhen:     nil,
				DeleteWhen:      nil,
				ArchiveOnDelete: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.celExpr)
		})
	}
}

func TestExtractNamespacesByKind(t *testing.T) {
	tests := []struct {
		name       string
		sinkFilter *kubearchivev1.SinkFilter
		expected   map[string]map[string]CelExpressions
	}{
		{
			name:       "Extract from test SinkFilter",
			sinkFilter: createTestSinkFilter(),
			expected: map[string]map[string]CelExpressions{
				"Pod-v1": {
					"test-namespace": CelExpressions{
						ArchiveWhen:     nil,
						DeleteWhen:      nil,
						ArchiveOnDelete: nil,
					},
				},
			},
		},
		{
			name: "Empty SinkFilter",
			sinkFilter: &kubearchivev1.SinkFilter{
				Spec: kubearchivev1.SinkFilterSpec{
					Namespaces: map[string][]kubearchivev1.KubeArchiveConfigResource{},
				},
			},
			expected: map[string]map[string]CelExpressions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNamespacesByKind(tt.sinkFilter, Controller)

			assert.Equal(t, len(tt.expected), len(result))
			for key := range tt.expected {
				assert.Contains(t, result, key)
				assert.Equal(t, len(tt.expected[key]), len(result[key]))
			}
		})
	}
}

func TestExtractNamespaceByKind(t *testing.T) {
	targetNamespace := "test-namespace"
	sinkFilter := createTestSinkFilter()

	result := ExtractNamespaceByKind(sinkFilter, targetNamespace, Controller)

	assert.Len(t, result, 1)
	for _, nsMap := range result {
		assert.Contains(t, nsMap, targetNamespace)
		celExpr := nsMap[targetNamespace]
		assert.NotNil(t, celExpr.ArchiveWhen)
		assert.NotNil(t, celExpr.DeleteWhen)
		assert.NotNil(t, celExpr.ArchiveOnDelete)
	}
}

func TestCompileCELExpression(t *testing.T) {
	tests := []struct {
		name           string
		expression     string
		expressionType string
		namespace      string
		expectNil      bool
	}{
		{
			name:           "Empty expression returns nil",
			expression:     "",
			expressionType: "ArchiveWhen",
			namespace:      "test",
			expectNil:      true,
		},
		{
			name:           "Valid expression compiles successfully",
			expression:     "true",
			expressionType: "ArchiveWhen",
			namespace:      "test",
			expectNil:      false,
		},
		{
			name:           "Complex valid expression",
			expression:     "metadata.name == 'test'",
			expressionType: "DeleteWhen",
			namespace:      "test",
			expectNil:      false,
		},
		{
			name:           "Invalid expression returns nil",
			expression:     "invalid syntax +++",
			expressionType: "ArchiveOnDelete",
			namespace:      "test",
			expectNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompileCELExpression(tt.expression, tt.expressionType, tt.namespace)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.IsType(t, (*cel.Program)(nil), result)
			}
		})
	}
}

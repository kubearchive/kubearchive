// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterKubeArchiveConfigCustomDefaulter(t *testing.T) {
	defaulter := ClusterKubeArchiveConfigCustomDefaulter{}
	kac := &ClusterKubeArchiveConfig{}
	err := defaulter.Default(context.Background(), kac)
	assert.NoError(t, err)
	assert.Equal(t, ClusterKubeArchiveConfigSpec{Resources: nil}, kac.Spec)
}

func TestClusterKubeArchiveConfigValidateName(t *testing.T) {
	k9eResourceName := "kubearchive"
	tests := []struct {
		name      string
		kacName   string
		validated bool
	}{
		{
			name:      "Valid name",
			kacName:   k9eResourceName,
			validated: true,
		},
		{
			name:      "Invalid name",
			kacName:   "otherName",
			validated: false,
		},
	}
	validator := ClusterKubeArchiveConfigCustomValidator{kubearchiveResourceName: k9eResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kac := &ClusterKubeArchiveConfig{ObjectMeta: metav1.ObjectMeta{Name: test.kacName}}
			warns, err := validator.ValidateCreate(context.Background(), kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.kacName)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &ClusterKubeArchiveConfig{}, kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.kacName)
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), kac)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

func TestClusterKubeArchiveConfigValidateCELExpression(t *testing.T) {
	k9eResourceName := "kubearchive"
	invalid := "status.state *^ Completed'"
	valid := "status.state == 'Completed'"
	tests := []struct {
		name            string
		archiveWhen     string
		deleteWhen      string
		archiveOnDelete string
		validated       bool
	}{
		{
			name:            "Invalid archiveWhen expression",
			archiveWhen:     invalid,
			deleteWhen:      valid,
			archiveOnDelete: valid,
			validated:       false,
		},
		{
			name:            "Invalid deleteWhen expression",
			archiveWhen:     valid,
			deleteWhen:      invalid,
			archiveOnDelete: valid,
			validated:       false,
		},
		{
			name:            "Invalid archiveOnDelete expression",
			archiveWhen:     valid,
			deleteWhen:      valid,
			archiveOnDelete: invalid,
			validated:       false,
		},
		{
			name:            "All expressions invalid",
			archiveWhen:     invalid,
			deleteWhen:      invalid,
			archiveOnDelete: invalid,
			validated:       false,
		},
		{
			name:            "All expressions valid",
			archiveWhen:     valid,
			deleteWhen:      valid,
			archiveOnDelete: valid,
			validated:       true,
		},
	}
	validator := ClusterKubeArchiveConfigCustomValidator{kubearchiveResourceName: k9eResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kac := &ClusterKubeArchiveConfig{
				ObjectMeta: metav1.ObjectMeta{Name: k9eResourceName},
				Spec: ClusterKubeArchiveConfigSpec{
					Resources: []KubeArchiveConfigResource{
						{
							ArchiveWhen:     test.archiveWhen,
							DeleteWhen:      test.deleteWhen,
							ArchiveOnDelete: test.archiveOnDelete,
						},
					}},
			}
			warns, err := validator.ValidateCreate(context.Background(), kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), "Syntax error")
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &ClusterKubeArchiveConfig{}, kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), "Syntax error")
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), kac)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

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

func TestClusterKubeArchiveConfigValidateDurationString(t *testing.T) {
	k9eResourceName := "kubearchive"
	tests := []struct {
		name            string
		archiveWhen     string
		deleteWhen      string
		archiveOnDelete string
		validated       bool
		expectedError   string
	}{
		{
			name:            "Valid duration string in archiveWhen",
			archiveWhen:     "duration('1h')",
			deleteWhen:      "",
			archiveOnDelete: "",
			validated:       true,
		},
		{
			name:            "Invalid duration string in archiveWhen",
			archiveWhen:     "duration('invalid')",
			deleteWhen:      "",
			archiveOnDelete: "",
			validated:       false,
			expectedError:   "invalid duration string 'duration('invalid')'",
		},
		{
			name:            "Invalid duration string in deleteWhen",
			archiveWhen:     "",
			deleteWhen:      "duration('bad-format')",
			archiveOnDelete: "",
			validated:       false,
			expectedError:   "invalid duration string 'duration('bad-format')'",
		},
		{
			name:            "Invalid duration string in archiveOnDelete",
			archiveWhen:     "",
			deleteWhen:      "",
			archiveOnDelete: "duration('xyz')",
			validated:       false,
			expectedError:   "invalid duration string 'duration('xyz')'",
		},
		{
			name:            "Multiple valid duration strings",
			archiveWhen:     "duration('1h') + duration('30m')",
			deleteWhen:      "",
			archiveOnDelete: "",
			validated:       true,
		},
		{
			name:            "Multiple duration strings with one invalid",
			archiveWhen:     "duration('1h') + duration('invalid-time')",
			deleteWhen:      "",
			archiveOnDelete: "",
			validated:       false,
			expectedError:   "invalid duration string 'duration('invalid-time')'",
		},
	}
	validator := ClusterKubeArchiveConfigCustomValidator{kubearchiveResourceName: k9eResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			ckac := &ClusterKubeArchiveConfig{
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
			warns, err := validator.ValidateCreate(context.Background(), ckac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &ClusterKubeArchiveConfig{}, ckac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
			}
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

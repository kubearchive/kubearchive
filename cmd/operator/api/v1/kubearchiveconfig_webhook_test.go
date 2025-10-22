// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKubeArchiveConfigCustomDefaulter(t *testing.T) {
	defaulter := KubeArchiveConfigCustomDefaulter{}
	kac := &KubeArchiveConfig{}
	err := defaulter.Default(context.Background(), kac)
	assert.NoError(t, err)
	assert.Equal(t, KubeArchiveConfigSpec{Resources: nil}, kac.Spec)
}

func TestKubeArchiveConfigValidateName(t *testing.T) {
	tests := []struct {
		name      string
		kacName   string
		validated bool
	}{
		{
			name:      "Valid name",
			kacName:   constants.KubeArchiveConfigResourceName,
			validated: true,
		},
		{
			name:      "Invalid name",
			kacName:   "otherName",
			validated: false,
		},
	}
	validator := KubeArchiveConfigCustomValidator{kubearchiveResourceName: constants.KubeArchiveConfigResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kac := &KubeArchiveConfig{ObjectMeta: metav1.ObjectMeta{Name: test.kacName}}
			warns, err := validator.ValidateCreate(context.Background(), kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.kacName)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &KubeArchiveConfig{}, kac)
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

func TestKubeArchiveConfigValidateNamespace(t *testing.T) {
	tests := []struct {
		name         string
		kacNamespace string
		validated    bool
	}{
		{
			name:         "Valid namespace",
			kacNamespace: "my-namespace",
			validated:    true,
		},
		{
			name:         "Invalid namespace",
			kacNamespace: constants.KubeArchiveNamespace,
			validated:    false,
		},
	}
	validator := KubeArchiveConfigCustomValidator{kubearchiveResourceName: constants.KubeArchiveConfigResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kac := &KubeArchiveConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.KubeArchiveConfigResourceName,
					Namespace: test.kacNamespace,
				},
			}

			warns, err := validator.ValidateCreate(context.Background(), kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "cannot create KubeArchiveConfig in the '%s' namespace", test.kacNamespace)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &KubeArchiveConfig{}, kac)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "cannot create KubeArchiveConfig in the '%s' namespace", test.kacNamespace)
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), kac)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

func TestKubeArchiveConfigValidateDurationString(t *testing.T) {
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
	validator := KubeArchiveConfigCustomValidator{kubearchiveResourceName: k9eResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kac := &KubeArchiveConfig{
				ObjectMeta: metav1.ObjectMeta{Name: k9eResourceName},
				Spec: KubeArchiveConfigSpec{
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
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &KubeArchiveConfig{}, kac)
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

func TestKubeArchiveConfigValidateCELExpression(t *testing.T) {
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
	validator := KubeArchiveConfigCustomValidator{kubearchiveResourceName: constants.KubeArchiveConfigResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kac := &KubeArchiveConfig{
				ObjectMeta: metav1.ObjectMeta{Name: constants.KubeArchiveConfigResourceName},
				Spec: KubeArchiveConfigSpec{
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
			warns, err = validator.ValidateUpdate(context.Background(), &KubeArchiveConfig{}, kac)
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

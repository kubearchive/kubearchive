// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKroniclerConfigCustomDefaulter(t *testing.T) {
	defaulter := KroniclerConfigCustomDefaulter{}
	kron := &KroniclerConfig{}
	err := defaulter.Default(context.Background(), kron)
	assert.NoError(t, err)
	assert.Equal(t, KroniclerConfigSpec{Resources: nil}, kron.Spec)
}

func TestKroniclerConfigValidateName(t *testing.T) {
	kroniclerResourceName := "kronicler"
	tests := []struct {
		name      string
		kronName  string
		validated bool
	}{
		{
			name:      "Valid name",
			kronName:  kroniclerResourceName,
			validated: true,
		},
		{
			name:      "Invalid name",
			kronName:  "otherName",
			validated: false,
		},
	}
	validator := KroniclerConfigCustomValidator{kroniclerResourceName: kroniclerResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kron := &KroniclerConfig{ObjectMeta: metav1.ObjectMeta{Name: test.kronName}}
			warns, err := validator.ValidateCreate(context.Background(), kron)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.kronName)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &KroniclerConfig{}, kron)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.kronName)
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), kron)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

func TestValidateCELExpression(t *testing.T) {
	kroniclerResourceName := "kronicler"
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
	validator := KroniclerConfigCustomValidator{kroniclerResourceName: kroniclerResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			kron := &KroniclerConfig{
				ObjectMeta: metav1.ObjectMeta{Name: kroniclerResourceName},
				Spec: KroniclerConfigSpec{
					Resources: []KroniclerConfigResource{
						{
							ArchiveWhen:     test.archiveWhen,
							DeleteWhen:      test.deleteWhen,
							ArchiveOnDelete: test.archiveOnDelete,
						},
					}},
			}
			warns, err := validator.ValidateCreate(context.Background(), kron)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), "Syntax error")
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &KroniclerConfig{}, kron)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), "Syntax error")
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), kron)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

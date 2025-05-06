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

func TestSinkFilterCustomDefaulter(t *testing.T) {
	defaulter := SinkFilterCustomDefaulter{}
	sf := &SinkFilter{}
	err := defaulter.Default(context.Background(), sf)
	assert.NoError(t, err)
	assert.Equal(t, SinkFilterSpec{Namespaces: nil}, sf.Spec)
}

func TestSinkFilterValidateName(t *testing.T) {
	tests := []struct {
		name      string
		sfName    string
		nsName    string
		validated bool
	}{
		{
			name:      "Valid name",
			sfName:    constants.SinkFilterResourceName,
			nsName:    constants.KubeArchiveNamespace,
			validated: true,
		},
		{
			name:      "Invalid name",
			sfName:    "otherName",
			nsName:    constants.KubeArchiveNamespace,
			validated: false,
		},
		{
			name:      "Valid namespace",
			sfName:    constants.SinkFilterResourceName,
			nsName:    constants.KubeArchiveNamespace,
			validated: true,
		},
		{
			name:      "Invalid namespace",
			sfName:    constants.SinkFilterResourceName,
			nsName:    "otherName",
			validated: false,
		},
	}
	validator := SinkFilterCustomValidator{sinkFilterResourceName: constants.SinkFilterResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			sf := &SinkFilter{ObjectMeta: metav1.ObjectMeta{Name: test.sfName, Namespace: test.nsName}}
			warns, err := validator.ValidateCreate(context.Background(), sf)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.sfName)
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &SinkFilter{}, sf)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Errorf(t, err, "invalid resource name %s", test.sfName)
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), sf)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

func TestSinkFilterValidateCELExpression(t *testing.T) {
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
	validator := SinkFilterCustomValidator{sinkFilterResourceName: constants.SinkFilterResourceName}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create resource
			sf := &SinkFilter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.SinkFilterResourceName,
					Namespace: constants.KubeArchiveNamespace,
				},
				Spec: SinkFilterSpec{
					Namespaces: map[string][]KubeArchiveConfigResource{"foo": {{
						ArchiveWhen:     test.archiveWhen,
						DeleteWhen:      test.deleteWhen,
						ArchiveOnDelete: test.archiveOnDelete,
					}},
					}},
			}
			warns, err := validator.ValidateCreate(context.Background(), sf)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), "Syntax error")
			}
			// Update resource
			warns, err = validator.ValidateUpdate(context.Background(), &SinkFilter{}, sf)
			assert.Nil(t, warns)
			if test.validated {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), "Syntax error")
			}
			// Delete resource
			warns, err = validator.ValidateDelete(context.Background(), sf)
			assert.Nil(t, warns)
			assert.NoError(t, err)
		})
	}
}

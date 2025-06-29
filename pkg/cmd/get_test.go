// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompleteAPI(t *testing.T) {
	testCases := []struct {
		name            string
		namespace       string
		args            []string
		expectedApiPath string
		isCore          bool
		output          string
	}{
		{
			name:            "core",
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/pods",
			isCore:          true,
		},
		{
			name:            "non-core",
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/jobs",
			isCore:          false,
		},
		{
			name:            "core namespaced",
			namespace:       "test",
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/namespaces/test/pods",
			isCore:          true,
		},
		{
			name:            "non-core namespaced",
			namespace:       "test",
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/namespaces/test/jobs",
			isCore:          false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := NewGetOptions()
			options.kubeFlags.Namespace = &tc.namespace

			err := options.Complete(tc.args)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedApiPath, options.APIPath)
			assert.Equal(t, tc.isCore, options.IsCoreResource)
			assert.Equal(t, tc.args[0], options.GroupVersion)
			assert.Equal(t, tc.args[1], options.Resource)
			assert.NotNil(t, options.RESTConfig)
		})
	}

}

func TestOutputOK(t *testing.T) {
	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
		isValid        bool
	}{
		{
			name:           "valid json",
			args:           []string{"-o", "json"},
			expectedOutput: "json",
			isValid:        true,
		},

		{
			name:           "valid yaml",
			args:           []string{"-o", "yaml"},
			expectedOutput: "yaml",
			isValid:        true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			options := NewGetOptions()
			options.Output = tc.args[1]
			err := options.Complete(tc.args)

			assert.NoError(t, err)
			assert.Equal(t, tc.isValid, options.IsValidOutput)
			assert.Equal(t, tc.expectedOutput, options.Output)
		})
	}
}

func TestOutputError(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		expectedErr string
		isValid     bool
	}{
		{
			name:        "invalid json",
			args:        []string{"-o", "jon"},
			expectedErr: "unable to match a printer suitable for the output format jon, allowed formats are: json, yaml",
			isValid:     false,
		},
		{
			name:        "invalid yaml",
			args:        []string{"-o", "aml"},
			expectedErr: "unable to match a printer suitable for the output format aml, allowed formats are: json, yaml",
			isValid:     false,
		},
		{
			name:        "empty output",
			args:        []string{"-o", "     "},
			expectedErr: "unable to match a printer suitable for the output format      , allowed formats are: json, yaml",
			isValid:     false,
		},
	}
	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {
			options := NewGetOptions()
			options.Output = tc.args[1]
			err := options.Complete(tc.args)
			t.Logf("error: %s", err)

			assert.Equal(t, tc.isValid, options.IsValidOutput)
			assert.EqualErrorf(t, err, tc.expectedErr, "expected error '%s', got '%s'", tc.expectedErr, err.Error())
		})
	}
}

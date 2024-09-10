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

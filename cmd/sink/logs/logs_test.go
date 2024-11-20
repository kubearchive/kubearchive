// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/files"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func loadData(t testing.TB, file string) *unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	ret := &unstructured.Unstructured{}
	err = json.Unmarshal(data, ret)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	return ret
}

func loadExpected(t testing.TB, file string) []models.LogTuple {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	expected := []models.LogTuple{}
	err = json.Unmarshal(data, &expected)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	return expected
}

func TestUrlBuilderUrls(t *testing.T) {
	splunkNoContainerNames := "testdata/splunk/no-container-names"
	splunkWithContainerNames := "testdata/splunk/container-names"
	esNoContainerNames := "testdata/es/no-container-names"
	esWithContainerNames := "testdata/es/container-names"

	tests := []struct {
		name             string
		loggingConfigDir string
		data             *unstructured.Unstructured
		expected         []models.LogTuple
	}{
		{
			name:             "splunk urls for pod with one container",
			loggingConfigDir: splunkNoContainerNames,
			data:             loadData(t, "testdata/test1-data.json"),
			expected:         loadExpected(t, "testdata/splunk-test1-urls.json"),
		},
		{
			name:             "splunk urls for pod with multiple containers",
			loggingConfigDir: splunkNoContainerNames,
			data:             loadData(t, "testdata/test2-data.json"),
			expected:         loadExpected(t, "testdata/splunk-test2-urls.json"),
		},
		{
			name:             "splunk urls for pod with one container with container name",
			loggingConfigDir: splunkWithContainerNames,
			data:             loadData(t, "testdata/test1-data.json"),
			expected:         loadExpected(t, "testdata/splunk-test1-urls.json"),
		},
		{
			name:             "splunk urls for pod with multiple containers with container name",
			loggingConfigDir: splunkWithContainerNames,
			data:             loadData(t, "testdata/test2-data.json"),
			expected:         loadExpected(t, "testdata/splunk-test2-urls.json"),
		},
		{
			name:             "elastic search urls for pod with one container",
			loggingConfigDir: esNoContainerNames,
			data:             loadData(t, "testdata/test1-data.json"),
			expected:         loadExpected(t, "testdata/es-test1-urls.json"),
		},
		{
			name:             "elastic search urls for pod with multiple containers",
			loggingConfigDir: esNoContainerNames,
			data:             loadData(t, "testdata/test2-data.json"),
			expected:         loadExpected(t, "testdata/es-test2-urls.json"),
		},
		{
			name:             "elastic search urls for pod with one container with container name",
			loggingConfigDir: esWithContainerNames,
			data:             loadData(t, "testdata/test1-data.json"),
			expected:         loadExpected(t, "testdata/es-test1-urls.json"),
		},
		{
			name:             "elastic search urls for pod with multiple containers with container name",
			loggingConfigDir: esWithContainerNames,
			data:             loadData(t, "testdata/test2-data.json"),
			expected:         loadExpected(t, "testdata/es-test2-urls.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(files.LoggingDirEnvVar, tt.loggingConfigDir)
			ub, err := NewUrlBuilder()
			if err != nil {
				assert.FailNowf(t, "could not create UrlBuilder", "%w", err)
			}
			res, err := ub.Urls(context.Background(), tt.data)
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestNewUrlBuilder(t *testing.T) {
	tests := []struct {
		name             string
		loggingConfigDir string
		err              bool
	}{
		{
			name:             "loggingConfigDir does not exist",
			loggingConfigDir: "fake/path",
			err:              true,
		},
		{
			name:             "loggingConfigDir is empty",
			loggingConfigDir: "testdata/emptyLoggingConfig",
			err:              true,
		},
		{
			name:             "UrlBuilder is created",
			loggingConfigDir: "testdata/splunk/no-container-names",
			err:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(files.LoggingDirEnvVar, tt.loggingConfigDir)
			ub, err := NewUrlBuilder()
			if tt.err {
				assert.NotNil(t, err)
				assert.Nil(t, ub)
			} else {
				assert.Nil(t, err)
				assert.NotNil(t, ub)
			}
		})
	}
}

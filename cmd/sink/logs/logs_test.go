// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func loadConfigMaps(t testing.TB, files ...string) []runtime.Object {
	t.Helper()
	configMaps := make([]runtime.Object, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			assert.FailNow(t, err.Error())
		}
		cm := &corev1.ConfigMap{}
		err = json.Unmarshal(data, &cm)
		if err != nil {
			assert.FailNow(t, err.Error())
		}
		configMaps = append(configMaps, cm)
	}
	return configMaps
}

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
	splunkCmFiles := []string{
		"testdata/kubearchive-config-splunk.json",
		"testdata/kubearchive-logging-splunk.json",
	}
	splunkCmWithContainerNameFiles := []string{
		"testdata/kubearchive-config-splunk.json",
		"testdata/kubearchive-logging-splunk-container-name.json",
	}
	esCmFiles := []string{
		"testdata/kubearchive-config-es.json",
		"testdata/kubearchive-logging-es.json",
	}
	esCmWithContainerNameFiles := []string{
		"testdata/kubearchive-config-es.json",
		"testdata/kubearchive-logging-es-container-name.json",
	}
	splunkConfigMaps := loadConfigMaps(t, splunkCmFiles...)
	splunkConfigMapsWithContainerName := loadConfigMaps(t, splunkCmWithContainerNameFiles...)
	esConfigMaps := loadConfigMaps(t, esCmFiles...)
	esConfigMapsWithContainerName := loadConfigMaps(t, esCmWithContainerNameFiles...)

	tests := []struct {
		name       string
		configMaps []runtime.Object
		data       *unstructured.Unstructured
		expected   []models.LogTuple
	}{
		{
			name:       "splunk urls for pod with one container",
			configMaps: splunkConfigMaps,
			data:       loadData(t, "testdata/test1-data.json"),
			expected:   loadExpected(t, "testdata/splunk-test1-urls.json"),
		},
		{
			name:       "splunk urls for pod with multiple containers",
			configMaps: splunkConfigMaps,
			data:       loadData(t, "testdata/test2-data.json"),
			expected:   loadExpected(t, "testdata/splunk-test2-urls.json"),
		},
		{
			name:       "splunk urls for pod with one container with container name",
			configMaps: splunkConfigMapsWithContainerName,
			data:       loadData(t, "testdata/test1-data.json"),
			expected:   loadExpected(t, "testdata/splunk-test1-urls.json"),
		},
		{
			name:       "splunk urls for pod with multiple containers with container name",
			configMaps: splunkConfigMapsWithContainerName,
			data:       loadData(t, "testdata/test2-data.json"),
			expected:   loadExpected(t, "testdata/splunk-test2-urls.json"),
		},
		{
			name:       "elastic search urls for pod with one container",
			configMaps: esConfigMaps,
			data:       loadData(t, "testdata/test1-data.json"),
			expected:   loadExpected(t, "testdata/es-test1-urls.json"),
		},
		{
			name:       "elastic search urls for pod with multiple containers",
			configMaps: esConfigMaps,
			data:       loadData(t, "testdata/test2-data.json"),
			expected:   loadExpected(t, "testdata/es-test2-urls.json"),
		},
		{
			name:       "elastic search urls for pod with one container with container name",
			configMaps: esConfigMapsWithContainerName,
			data:       loadData(t, "testdata/test1-data.json"),
			expected:   loadExpected(t, "testdata/es-test1-urls.json"),
		},
		{
			name:       "elastic search urls for pod with multiple containers with container name",
			configMaps: esConfigMapsWithContainerName,
			data:       loadData(t, "testdata/test2-data.json"),
			expected:   loadExpected(t, "testdata/es-test2-urls.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(tt.configMaps...)
			ub, err := NewUrlBuilder(context.Background(), client)
			if err != nil {
				assert.FailNowf(t, "could not create UrlBuilder", "%w", err)
			}
			res, err := ub.Urls(context.Background(), tt.data)
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, res)
		})
	}
}

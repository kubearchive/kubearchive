// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package logurls

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGenerateLogURLs(t *testing.T) {
	testCases := []struct {
		name     string
		cm       string
		data     string
		expected string
	}{
		{
			name:     "splunk test generate log URLs single container",
			cm:       "testdata/splunk-cm.json",
			data:     "testdata/test1-data.json",
			expected: "testdata/splunk-test1-urls.json",
		},
		{
			name:     "splunk test generate log URLs multiple containers",
			cm:       "testdata/splunk-cm.json",
			data:     "testdata/test2-data.json",
			expected: "testdata/splunk-test2-urls.json",
		},
		{
			name:     "elasticsearch test generate log URLs single container",
			cm:       "testdata/es-cm.json",
			data:     "testdata/test1-data.json",
			expected: "testdata/es-test1-urls.json",
		},
		{
			name:     "elasticsearch test generate log URLs multiple containers",
			cm:       "testdata/es-cm.json",
			data:     "testdata/test2-data.json",
			expected: "testdata/es-test2-urls.json",
		},
		{
			name:     "loki test generate log URLs multiple containers",
			cm:       "testdata/loki-cm.json",
			data:     "testdata/test3-data.json",
			expected: "testdata/loki-test3-urls.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm := compileCELExpressions(loadJSON[map[string]any](tc.cm))
			data := loadJSON[*unstructured.Unstructured](tc.data)
			expected := loadJSON[[]models.LogTuple](tc.expected)
			result, _ := GenerateLogURLs(context.Background(), cm, data)
			assert.Equal(t, expected, result, "Does not match")
		})
	}
}

func compileCELExpressions(cm map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range cm {
		if strings.HasPrefix(value.(string), "cel:") {
			value, _ = ocel.CompileCELExpr(value.(string)[4:])
		}
		result[key] = value
	}
	return result
}

func loadJSON[T any](filename string) T {
	var result T
	file, err := os.Open(filename)
	if err != nil {
		return result
	}
	defer file.Close()
	bytes, _ := io.ReadAll(file)

	if err := json.Unmarshal([]byte(bytes), &result); err != nil {
		return result
	}

	return result
}

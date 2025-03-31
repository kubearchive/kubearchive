// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0
package logurls

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	ocel "github.com/kronicler/kronicler/pkg/cel"
	"github.com/kronicler/kronicler/pkg/models"
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm := compileCELExpressions(loadMapJSON(tc.cm))
			data := loadUnstructuredJSON(tc.data)
			expected := loadExpectedJSON(tc.expected)
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

func loadMapJSON(filename string) map[string]interface{} {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	bytes, _ := io.ReadAll(file)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(bytes), &result); err != nil {
		return nil
	}

	return result
}

func loadUnstructuredJSON(filename string) *unstructured.Unstructured {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	bytes, _ := io.ReadAll(file)

	var result *unstructured.Unstructured
	if err := json.Unmarshal([]byte(bytes), &result); err != nil {
		return nil
	}
	return result
}

func loadExpectedJSON(filename string) []models.LogTuple {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	bytes, _ := io.ReadAll(file)

	var result []models.LogTuple
	if err := json.Unmarshal([]byte(bytes), &result); err != nil {
		return nil
	}
	return result
}

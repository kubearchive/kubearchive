// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package log_urls

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGenerateLogURLs(t *testing.T) {
	testCases := []struct {
		name     string
		cm       string
		data     string
		expected any
	}{
		{
			name:     "test generate log URLs single container",
			cm:       "testdata/splunk-cm.json",
			data:     "testdata/splunk-test1-data.json",
			expected: []string{"http://127.0.0.1:8111/app/search/search?q=search * | spath \"kubernetes.pod_id\" | search \"kubernetes.pod_id\"=\"4df2df3f-7397-4a63-86b4-9f5f1ff84f99\" | spath \"kubernetes.container_name\" | search \"kubernetes.container_name\"=\"generate\""},
		},
		{
			name:     "test generate log URLs multiple containers",
			cm:       "testdata/splunk-cm.json",
			data:     "testdata/splunk-test2-data.json",
			expected: []string{"http://127.0.0.1:8111/app/search/search?q=search * | spath \"kubernetes.pod_id\" | search \"kubernetes.pod_id\"=\"a8c9b834-bc4a-4438-8841-23a5ffb9164c\" | spath \"kubernetes.container_name\" | search \"kubernetes.container_name\"=\"nginx-container\"", "http://127.0.0.1:8111/app/search/search?q=search * | spath \"kubernetes.pod_id\" | search \"kubernetes.pod_id\"=\"a8c9b834-bc4a-4438-8841-23a5ffb9164c\" | spath \"kubernetes.container_name\" | search \"kubernetes.container_name\"=\"busybox-container\"", "http://127.0.0.1:8111/app/search/search?q=search * | spath \"kubernetes.pod_id\" | search \"kubernetes.pod_id\"=\"a8c9b834-bc4a-4438-8841-23a5ffb9164c\" | spath \"kubernetes.container_name\" | search \"kubernetes.container_name\"=\"echo-3\"", "http://127.0.0.1:8111/app/search/search?q=search * | spath \"kubernetes.pod_id\" | search \"kubernetes.pod_id\"=\"a8c9b834-bc4a-4438-8841-23a5ffb9164c\" | spath \"kubernetes.container_name\" | search \"kubernetes.container_name\"=\"echo-4\""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm := compileCELExpressions(loadJSON(tc.cm))
			data := loadJSONx(tc.data)
			result, _ := GenerateLogURLs(context.Background(), cm, data)
			assert.Equal(t, tc.expected, result, "Does not match")
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

func loadJSON(filename string) map[string]interface{} {
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

func loadJSONx(filename string) *unstructured.Unstructured {
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

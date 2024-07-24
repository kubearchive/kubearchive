// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package jsonpath

import (
	"fmt"
	"regexp"

	"k8s.io/client-go/util/jsonpath"
)

// Evaluates the JSON Path, path, against the json, data, using jsonpath from k8s.io/client-go/util and returns a bool
// representing whether or not the JSON Path exists in data. It expects find only one result. If the evaluation fails,
// it returns the error
func PathExists(path string, data map[string]interface{}) (bool, error) {
	jp := jsonpath.New("Parser")
	err := jp.Parse(path)
	if err != nil {
		return false, err
	}
	results, err := jp.FindResults(data)
	if err != nil {
		return false, err
	}
	if len(results) != 1 || len(results[0]) != 1 {
		return false, fmt.Errorf("Failed to find a match, expected one result from JSON Path")
	}
	return true, nil
}

// Copied from https://github.com/kubernetes/kubectl/blob/a70106d6a8b4fc24633f7020b9fdc416648e7f22/pkg/cmd/get/customcolumn.go#L38-L67
// Copyright 2014 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
var jsonRegexp = regexp.MustCompile(`^\{\.?([^{}]+)}$|^\.?([^{}]+)$`)

// relaxedJSONPathExpression attempts to be flexible with JSONPath expressions, it accepts:
//   - metadata.name (no leading '.' or curly braces '{...}'
//   - {metadata.name} (no leading '.')
//   - .metadata.name (no curly braces '{...}')
//   - {.metadata.name} (complete expression)
//
// And transforms them all into a valid jsonpath expression:
//
//	{.metadata.name}
func RelaxedJSONPathExpression(pathExpression string) (string, error) {
	if len(pathExpression) == 0 {
		return pathExpression, nil
	}
	submatches := jsonRegexp.FindStringSubmatch(pathExpression)
	if submatches == nil {
		return "", fmt.Errorf("unexpected path string, expected a 'name1.name2' or '.name1.name2' or '{name1.name2}' or '{.name1.name2}'")
	}
	if len(submatches) != 3 {
		return "", fmt.Errorf("unexpected submatch list: %v", submatches)
	}
	var fieldSpec string
	if len(submatches[1]) != 0 {
		fieldSpec = submatches[1]
	} else {
		fieldSpec = submatches[2]
	}
	return fmt.Sprintf("{.%s}", fieldSpec), nil
}

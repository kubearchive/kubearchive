// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package log_urls

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/traits"
	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	VARIABLE_REGEX string = "\\{([A-Za-z0-9_]+)\\}"
	LOG_URL        string = "LOG_URL"
)

func GenerateLogURLs(ctx context.Context, cm map[string]interface{}, data *unstructured.Unstructured) ([]string, error) {
	urls := []string{}
	r, err := regexp.Compile(VARIABLE_REGEX)
	if err != nil {
		return urls, fmt.Errorf("Could not compile Regex: %w", err)
	}
	// Generate a new map with any CEL expressions evaluated.
	m := make(map[string]interface{})
	for key, value := range cm {
		prgmPtr, ok := value.(*cel.Program)
		if ok {
			// We have a CEL expression. Evaluate it and use that value.
			value = ocel.ExecuteCEL(ctx, *prgmPtr, data)
		}
		m[key] = value
	}

	var vmaps = generateSubstitutionMaps(m)
	for _, vmap := range vmaps {
		urls = append(urls, interpolate(vmap[LOG_URL], vmap, r))
	}
	return urls, nil
}

func generateSubstitutionMaps(m map[string]interface{}) []map[string]string {
	// Generate individual maps for all the unique values. Each value in the original
	// map should be either a string or a list. Blow out the lists into individual maps,
	// one for each list item.
	var vmaps []map[string]string
	vmaps = append(vmaps, map[string]string{})
	for key, value := range m {
		list, ok := value.(traits.Lister)
		if ok {
			// Handle list.
			if list.Size().Value() == 0 {
				// Empty list, just set to the empty string.
				for _, vm := range vmaps {
					vm[key] = ""
				}
			} else {
				// Create a new map for each value in the list.
				var nmaps []map[string]string
				iterator := list.Iterator()
				for iterator.HasNext().Value().(bool) {
					val := iterator.Next().Value()
					for _, vm := range vmaps {
						nm := maps.Clone(vm)
						nm[key] = fmt.Sprintf("%v", val)
						nmaps = append(nmaps, nm)
					}
				}
				vmaps = nmaps
			}
		} else {
			for _, vm := range vmaps {
				vm[key] = fmt.Sprintf("%v", value)
			}
		}
	}
	return vmaps
}

func interpolate(val string, env map[string]string, r *regexp.Regexp) string {
	matches := r.FindAllStringSubmatch(val, -1)
	if matches == nil {
		// Finished, nothing more to substitute.
		return val
	}

	for _, m := range matches {
		val = strings.ReplaceAll(val, string(m[0]), env[string(m[1])])
	}
	return interpolate(val, env, r)
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package logurls

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/traits"
	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/models"
	"go.opentelemetry.io/otel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	variableRegex string = "\\{([A-Za-z0-9_]+)\\}"
	LogURL        string = "LOG_URL"
	ContainerName string = "CONTAINER_NAME"
	QueryKey      string = "QUERY"
	StartKey      string = "START"
	EndKey        string = "END"
)

func GenerateLogURLs(ctx context.Context, cm map[string]interface{}, data *unstructured.Unstructured) ([]models.LogTuple, error) {
	tracer := otel.Tracer("kubearchive")
	ctx, span := tracer.Start(ctx, "GenerateLogURLs")
	defer span.End()

	urls := []models.LogTuple{}
	r, err := regexp.Compile(variableRegex)
	if err != nil {
		return urls, fmt.Errorf("could not compile Regex: %w", err)
	}
	// Generate a new map with any CEL expressions evaluated.
	m := make(map[string]interface{})
	for key, value := range cm {
		prgmPtr, ok := value.(*cel.Program)
		if ok {
			// We have a CEL expression. Evaluate it and use that value.
			value, _ = ocel.ExecuteCEL(ctx, prgmPtr, data)
		}
		m[key] = value
	}

	var vmaps = generateSubstitutionMaps(m)
	for _, vmap := range vmaps {
		url := interpolateString(vmap[LogURL], vmap, r)
		query := interpolateString(vmap[QueryKey], vmap, r)
		start := vmap[StartKey]
		end := vmap[EndKey]
		urls = append(urls, models.LogTuple{
			ContainerName: vmap[ContainerName],
			Url:           url,
			Query:         query,
			Start:         start,
			End:           end,
		})
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

func interpolateString(val string, env map[string]string, r *regexp.Regexp) string {
	matches := r.FindAllStringSubmatch(val, -1)
	if matches == nil {
		// Finished, nothing more to substitute.
		return val
	}

	for _, m := range matches {
		val = strings.ReplaceAll(val, string(m[0]), env[string(m[1])])
	}
	return interpolateString(val, env, r)
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8sclient

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

// InstrumentedRoundTripper is a custom RoundTripper that adds custom attributes
// to http roundtripper
type InstrumentedRoundTripper struct {
	base http.RoundTripper
}

func NewInstrumentedRoundTripper(base http.RoundTripper) *InstrumentedRoundTripper {
	return &InstrumentedRoundTripper{base: base}
}

func (rt *InstrumentedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return otelhttp.NewTransport(rt.base,
		otelhttp.WithMetricAttributesFn(func(r *http.Request) []attribute.KeyValue {
			reqRouteType := ExtractKubernetesRouteType(r.URL.Path)
			return []attribute.KeyValue{
				attribute.String("route", reqRouteType),
			}
		}),
	).RoundTrip(req)
}

// ExtractKubernetesRouteType determines the type of Kubernetes API interaction
// based on the request path to provide low-cardinality route attributes.
func ExtractKubernetesRouteType(path string) string {
	if strings.Contains(path, "/api") || strings.Contains(path, "/apis") {
		// Discovery patterns (exact matches only)
		if path == "/api" || path == "/apis" {
			return "discovery.api_groups"
		}
		if path == "/api/v1" {
			return "discovery.core_resources"
		}
		if strings.HasPrefix(path, "/apis/") && strings.Count(path, "/") == 3 {
			return "discovery.group_resources"
		}

		// Handle namespaced core resources
		if strings.Contains(path, "/api/v1/namespaces/") {
			kind := extractResourceFromPath(path, "/api/v1/namespaces/")
			if kind != "" {
				return "namespaced.core." + kind
			}
		}

		// Handle cluster-wide core resources
		if strings.HasPrefix(path, "/api/v1/") && !strings.Contains(path, "namespaces") {
			kind := extractResourceFromPath(path, "/api/v1/")
			if kind != "" {
				return "cluster.core." + kind
			}
		}

		// Handle namespaced group resources
		if strings.Contains(path, "/apis/") && strings.Contains(path, "/namespaces/") {
			group, kind := extractGroupAndKind(path)
			if group != "" && kind != "" {
				return "namespaced." + group + "." + kind
			}
		}

		// Handle cluster-wide group resources
		if strings.HasPrefix(path, "/apis/") && !strings.Contains(path, "/namespaces/") {
			group, kind := extractGroupAndKind(path)
			if group != "" && kind != "" {
				return "cluster." + group + "." + kind
			}
		}

		return "discovery.other"
	}
	return "unknown"
}

func extractResourceFromPath(path, prefix string) string {
	if idx := strings.Index(path, prefix); idx != -1 {
		remaining := path[idx+len(prefix):]
		if strings.Contains(prefix, "namespaces/") {
			parts := strings.Split(remaining, "/")
			if len(parts) >= 2 {
				return parts[1]
			}
		} else {
			parts := strings.Split(remaining, "/")
			if len(parts) > 0 && parts[0] != "" {
				return parts[0]
			}
		}
	}
	return ""
}

func extractGroupAndKind(path string) (string, string) {
	if idx := strings.Index(path, "/apis/"); idx != -1 {
		remaining := path[idx+6:]
		parts := strings.Split(remaining, "/")

		if strings.Contains(path, "/namespaces/") {
			// Format: /apis/group/version/namespaces/namespace/kind
			if len(parts) >= 5 {
				group := parts[0]
				kind := parts[4]
				return group, kind
			}
		} else {
			// Format: /apis/group/version/kind
			if len(parts) >= 3 {
				group := parts[0]
				kind := parts[2]
				return group, kind
			}
		}
	}
	return "", ""
}

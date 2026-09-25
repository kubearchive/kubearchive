// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8sclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

// TestRoundTripNoLabelerInContext exercises the fallback branch in RoundTrip
// where no Labeler is pre-injected in the request context. RoundTrip must
// create and inject its own Labeler, complete the request successfully, and
// populate the route attribute on the internally created Labeler.
func TestRoundTripNoLabelerInContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewInstrumentedRoundTripper(http.DefaultTransport)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/nodes", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Confirm no Labeler is in the context before the call.
	if _, ok := otelhttp.LabelerFromContext(req.Context()); ok {
		t.Fatal("expected no Labeler in context before RoundTrip")
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Note: we cannot verify the route attribute on the internally created Labeler
	// from outside RoundTrip. When no Labeler is present in the context, RoundTrip
	// creates one and injects it into a new request via req.WithContext — but that
	// reassignment is local to RoundTrip and the caller's req is unchanged.
	// Attribute population via labeler.Add is covered by TestRoundTrip, which
	// exercises the same code path with a pre-injected Labeler that is accessible
	// to the test after the call.
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantRoute string
	}{
		{
			name:      "namespaced pods",
			path:      "/api/v1/namespaces/default/pods",
			wantRoute: "namespaced.core.pods",
		},
		{
			name:      "cluster nodes",
			path:      "/api/v1/nodes",
			wantRoute: "cluster.core.nodes",
		},
		{
			name:      "unknown path",
			path:      "/healthz",
			wantRoute: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// httptest.NewServer starts a real local HTTP server on a random port.
			// It always returns 200 regardless of path — we only need it to accept
			// the connection so RoundTrip can complete successfully.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			transport := NewInstrumentedRoundTripper(http.DefaultTransport)

			// Pre-inject a Labeler into the request context so we can inspect
			// the attributes added by RoundTrip after the call completes.
			labeler := &otelhttp.Labeler{}
			ctx := otelhttp.ContextWithLabeler(context.Background(), labeler)

			// Append tt.path to the server's base URL (e.g. http://127.0.0.1:PORT/api/v1/nodes)
			// so that RoundTrip reads the correct path when computing the route attribute.
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", resp.StatusCode)
			}

			// Verify the Labeler was populated with the correct route attribute.
			attrs := labeler.Get()
			var routeVal string
			for _, a := range attrs {
				if a.Key == attribute.Key("route") {
					routeVal = a.Value.AsString()
				}
			}
			if routeVal != tt.wantRoute {
				t.Errorf("route attribute = %q, want %q", routeVal, tt.wantRoute)
			}
		})
	}
}

func TestExtractKubernetesRouteType(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "token review",
			path: "/apis/authentication.k8s.io/v1/tokenreviews",
			want: "cluster.authentication.k8s.io.tokenreviews",
		},
		{
			name: "subject access review",
			path: "/apis/authorization.k8s.io/v1/subjectaccessreviews",
			want: "cluster.authorization.k8s.io.subjectaccessreviews",
		},
		{
			name: "api groups discovery",
			path: "/apis",
			want: "discovery.api_groups",
		},
		{
			name: "core api discovery",
			path: "/api",
			want: "discovery.api_groups",
		},
		{
			name: "core resources discovery",
			path: "/api/v1",
			want: "discovery.core_resources",
		},
		{
			name: "group resources discovery",
			path: "/apis/apps/v1",
			want: "discovery.group_resources",
		},
		{
			name: "get namespaced pods",
			path: "/api/v1/namespaces/default/pods",
			want: "namespaced.core.pods",
		},
		{
			name: "create namespaced service",
			path: "/api/v1/namespaces/kube-system/services",
			want: "namespaced.core.services",
		},
		{
			name: "update specific pod",
			path: "/api/v1/namespaces/default/pods/my-pod",
			want: "namespaced.core.pods",
		},
		{
			name: "delete specific pod",
			path: "/api/v1/namespaces/default/pods/my-pod",
			want: "namespaced.core.pods",
		},
		{
			name: "get cluster-wide nodes",
			path: "/api/v1/nodes",
			want: "cluster.core.nodes",
		},
		{
			name: "patch persistent volume",
			path: "/api/v1/persistentvolumes",
			want: "cluster.core.persistentvolumes",
		},
		{
			name: "get specific node",
			path: "/api/v1/nodes/worker-1",
			want: "cluster.core.nodes",
		},
		{
			name: "list namespaced deployments",
			path: "/apis/apps/v1/namespaces/default/deployments",
			want: "namespaced.apps.deployments",
		},
		{
			name: "create deployment",
			path: "/apis/apps/v1/namespaces/production/deployments/web-app",
			want: "namespaced.apps.deployments",
		},
		{
			name: "delete custom resource",
			path: "/apis/custom.io/v1/customresources",
			want: "cluster.custom.io.customresources",
		},
		{
			name: "patch cluster role",
			path: "/apis/rbac.authorization.k8s.io/v1/clusterroles",
			want: "cluster.rbac.authorization.k8s.io.clusterroles",
		},
		{
			name: "get namespaced ingress",
			path: "/apis/networking.k8s.io/v1/namespaces/default/ingresses",
			want: "namespaced.networking.k8s.io.ingresses",
		},
		{
			name: "update storage class",
			path: "/apis/storage.k8s.io/v1/storageclasses",
			want: "cluster.storage.k8s.io.storageclasses",
		},
		{
			name: "get configmaps",
			path: "/api/v1/namespaces/default/configmaps",
			want: "namespaced.core.configmaps",
		},
		{
			name: "list secrets",
			path: "/api/v1/namespaces/kube-system/secrets",
			want: "namespaced.core.secrets",
		},
		{
			name: "unknown path",
			path: "/something/else",
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractKubernetesRouteType(tt.path)
			if got != tt.want {
				t.Errorf("ExtractKubernetesRouteType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

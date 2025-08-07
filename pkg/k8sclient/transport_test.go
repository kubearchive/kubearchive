// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8sclient

import (
	"testing"
)

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

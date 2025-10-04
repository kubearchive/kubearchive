// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SinkFilterController", func() {
	Context("When reconciling a SinkFilter resource", func() {
		const (
			SinkFilterName      = "test-sinkfilter"
			SinkFilterNamespace = "default"
		)

		It("Should extract unique kinds correctly", func() {
			// Create a SinkFilter with multiple resources across different namespaces
			sinkFilter := &kubearchivev1.SinkFilter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SinkFilterName,
					Namespace: SinkFilterNamespace,
				},
				Spec: kubearchivev1.SinkFilterSpec{
					Namespaces: map[string][]kubearchivev1.KubeArchiveConfigResource{
						constants.SinkFilterGlobalNamespace: {
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "apps/v1",
									Kind:       "Deployment",
								},
								ArchiveWhen: "true",
							},
						},
						"test-namespace1": {
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "v1",
									Kind:       "Pod",
								},
								ArchiveWhen: "true",
							},
						},
						"test-namespace2": {
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "v1",
									Kind:       "Pod",
								},
								ArchiveWhen: "true",
							},
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "v1",
									Kind:       "Service",
								},
								ArchiveWhen: "true",
							},
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			kubearchivev1.AddToScheme(scheme) //nolint:errcheck

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &SinkFilterReconciler{
				Client: client,
				Scheme: scheme,
				// cloudEventPublisher is nil in tests - controller handles this gracefully
			}

			// Test extractNamespacesByKinds
			namespacesByKinds := reconciler.extractNamespacesByKinds(sinkFilter)

			// Should have 3 unique kinds: Deployment-apps/v1, Pod-v1, Service-v1
			Expect(len(namespacesByKinds)).To(Equal(3))

			expectedNamespaces := map[string]map[string]struct{}{
				"Deployment-apps/v1": {constants.SinkFilterGlobalNamespace: struct{}{}},              // Deployment is global
				"Pod-v1":             {"test-namespace1": struct{}{}, "test-namespace2": struct{}{}}, // Pod is in both namespaces
				"Service-v1":         {"test-namespace2": struct{}{}},                                // Service is only in test-namespace2
			}

			for key, expectedNsMap := range expectedNamespaces {
				actualNsMap, exists := namespacesByKinds[key]
				Expect(exists).To(BeTrue(), "Expected kind %s to exist", key)
				Expect(len(actualNsMap)).To(Equal(len(expectedNsMap)))
				for expectedNs := range expectedNsMap {
					_, exists := actualNsMap[expectedNs]
					Expect(exists).To(BeTrue(), "Expected namespace %s to exist for kind %s", expectedNs, key)
				}
			}
		})

		It("Should handle empty SinkFilter correctly", func() {
			sinkFilter := &kubearchivev1.SinkFilter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SinkFilterName,
					Namespace: SinkFilterNamespace,
				},
				Spec: kubearchivev1.SinkFilterSpec{
					Namespaces: map[string][]kubearchivev1.KubeArchiveConfigResource{},
				},
			}

			scheme := runtime.NewScheme()
			kubearchivev1.AddToScheme(scheme) //nolint:errcheck

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &SinkFilterReconciler{
				Client: client,
				Scheme: scheme,
				// cloudEventPublisher is nil in tests - controller handles this gracefully
			}

			namespacesByKinds := reconciler.extractNamespacesByKinds(sinkFilter)
			Expect(len(namespacesByKinds)).To(Equal(0))
		})

		It("Should handle selective watch updates efficiently", func() {
			reconciler := &SinkFilterReconciler{
				watches: make(map[string]*WatchInfo),
				// cloudEventPublisher is nil in tests - controller handles this gracefully
			}

			// Simulate initial state with some existing watches
			reconciler.watches["Deployment-apps/v1"] = &WatchInfo{
				StopCh: make(chan struct{}),
			}
			reconciler.watches["Pod-v1"] = &WatchInfo{
				StopCh: make(chan struct{}),
			}

			// Test finding watches to stop
			newNamespacesByKinds := map[string]map[string]struct{}{
				"Deployment-apps/v1": {"test-namespace1": struct{}{}},                                // Keep
				"Service-v1":         {"test-namespace1": struct{}{}, "test-namespace2": struct{}{}}, // New
				// Pod-v1 is missing, so should be stopped
			}

			toStop := reconciler.findWatchesToStop(newNamespacesByKinds)
			Expect(len(toStop)).To(Equal(1))
			_, exists := toStop["Pod-v1"]
			Expect(exists).To(BeTrue())

			// Test finding watches to create
			toCreate := reconciler.findWatchesToCreate(newNamespacesByKinds)
			Expect(len(toCreate)).To(Equal(1))
			_, exists = toCreate["Service-v1"]
			Expect(exists).To(BeTrue())

			// Verify unchanged watches are identified correctly
			unchanged := 0
			for key := range reconciler.watches {
				if _, stillNeeded := newNamespacesByKinds[key]; stillNeeded {
					unchanged++
				}
			}
			Expect(unchanged).To(Equal(1)) // Only Deployment should remain unchanged
		})
	})
})

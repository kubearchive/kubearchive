// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/filters"
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
								Selector: kubearchivev1.APIVersionKind{
									APIVersion: "apps/v1",
									Kind:       "Deployment",
								},
								ArchiveWhen: "true",
							},
						},
						"test-namespace1": {
							{
								Selector: kubearchivev1.APIVersionKind{
									APIVersion: "v1",
									Kind:       "Pod",
								},
								ArchiveWhen: "true",
							},
						},
						"test-namespace2": {
							{
								Selector: kubearchivev1.APIVersionKind{
									APIVersion: "v1",
									Kind:       "Pod",
								},
								ArchiveWhen: "true",
							},
							{
								Selector: kubearchivev1.APIVersionKind{
									APIVersion: "v1",
									Kind:       "Service",
								},
								ArchiveWhen: "true",
							},
						},
					},
				},
			}

			// Test ExtractAllNamespacesByKinds
			namespacesByKinds := filters.ExtractAllNamespacesByKinds(sinkFilter)

			// Should have 3 unique kinds: Deployment-apps/v1, Pod-v1, Service-v1
			Expect(len(namespacesByKinds)).To(Equal(3))

			expectedNamespaces := map[string][]string{
				"Deployment-apps/v1": {constants.SinkFilterGlobalNamespace},  // Deployment is global
				"Pod-v1":             {"test-namespace1", "test-namespace2"}, // Pod is in both namespaces
				"Service-v1":         {"test-namespace2"},                    // Service is only in test-namespace2
			}

			for key, expectedNsList := range expectedNamespaces {
				actualNsMap, exists := namespacesByKinds[key]
				Expect(exists).To(BeTrue(), "Expected kind %s to exist", key)
				Expect(len(actualNsMap)).To(Equal(len(expectedNsList)))
				for _, expectedNs := range expectedNsList {
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

			namespacesByKinds := filters.ExtractAllNamespacesByKinds(sinkFilter)
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
			newNamespacesByKinds := map[string]map[string]filters.CelExpressions{
				"Deployment-apps/v1": {"test-namespace1": filters.CelExpressions{}},                                              // Keep
				"Service-v1":         {"test-namespace1": filters.CelExpressions{}, "test-namespace2": filters.CelExpressions{}}, // New
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

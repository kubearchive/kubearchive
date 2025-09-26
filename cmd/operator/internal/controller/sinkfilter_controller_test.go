// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
						"namespace1": {
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "apps/v1",
									Kind:       "Deployment",
								},
								ArchiveWhen: "true",
							},
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "v1",
									Kind:       "Pod",
								},
								ArchiveWhen: "true",
							},
						},
						"namespace2": {
							{
								Selector: sourcesv1.APIVersionKindSelector{
									APIVersion: "apps/v1",
									Kind:       "Deployment", // Duplicate - should be deduplicated
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

			// Test extractUniqueKinds
			uniqueKinds := reconciler.extractUniqueKinds(sinkFilter)

			// Should have 3 unique kinds: Deployment-apps/v1, Pod-v1, Service-v1
			Expect(len(uniqueKinds)).To(Equal(3))

			expectedKinds := map[string]sourcesv1.APIVersionKindSelector{
				"Deployment-apps/v1": {Kind: "Deployment", APIVersion: "apps/v1"},
				"Pod-v1":             {Kind: "Pod", APIVersion: "v1"},
				"Service-v1":         {Kind: "Service", APIVersion: "v1"},
			}

			for key, expected := range expectedKinds {
				actual, exists := uniqueKinds[key]
				Expect(exists).To(BeTrue(), "Expected kind %s to exist", key)
				Expect(actual.Kind).To(Equal(expected.Kind))
				Expect(actual.APIVersion).To(Equal(expected.APIVersion))
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

			uniqueKinds := reconciler.extractUniqueKinds(sinkFilter)
			Expect(len(uniqueKinds)).To(Equal(0))
		})

		It("Should create proper resource predicate with global namespace support", func() {
			// Create test namespaces configuration with both global and specific namespace resources
			namespaces := map[string][]kubearchivev1.KubeArchiveConfigResource{
				constants.SinkFilterGlobalNamespace: {
					{
						Selector: sourcesv1.APIVersionKindSelector{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
				},
				"test-namespace": {
					{
						Selector: sourcesv1.APIVersionKindSelector{
							APIVersion: "v1",
							Kind:       "Pod",
						},
					},
				},
			}

			reconciler := &SinkFilterReconciler{
				// cloudEventPublisher is nil in tests - controller handles this gracefully
			}

			// Test global resource - should return true for any namespace
			deploymentKindSelector := sourcesv1.APIVersionKindSelector{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}
			deploymentPredicate := reconciler.createResourcePredicate(namespaces, deploymentKindSelector)

			// Deployment in test-namespace - should return true because it's globally configured
			deploymentInTestNs := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deployment",
						"namespace": "test-namespace",
					},
				},
			}
			Expect(deploymentPredicate(deploymentInTestNs)).To(BeTrue())

			// Deployment in different namespace - should return true because it's globally configured
			deploymentInOtherNs := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "other-deployment",
						"namespace": "other-namespace",
					},
				},
			}
			Expect(deploymentPredicate(deploymentInOtherNs)).To(BeTrue())

			// Test namespace-specific resource
			podKindSelector := sourcesv1.APIVersionKindSelector{
				APIVersion: "v1",
				Kind:       "Pod",
			}
			podPredicate := reconciler.createResourcePredicate(namespaces, podKindSelector)

			// Pod in test-namespace - should return true because it's configured for that namespace
			podInTestNs := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "test-pod",
						"namespace": "test-namespace",
					},
				},
			}
			Expect(podPredicate(podInTestNs)).To(BeTrue())

			// Pod in different namespace - should return false because it's only configured for test-namespace
			podInOtherNs := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "other-pod",
						"namespace": "other-namespace",
					},
				},
			}
			Expect(podPredicate(podInOtherNs)).To(BeFalse())

			// Test unconfigured resource type
			serviceKindSelector := sourcesv1.APIVersionKindSelector{
				APIVersion: "v1",
				Kind:       "Service",
			}
			servicePredicate := reconciler.createResourcePredicate(namespaces, serviceKindSelector)

			// Service in any namespace - should return false because it's not configured anywhere
			serviceInTestNs := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]interface{}{
						"name":      "test-service",
						"namespace": "test-namespace",
					},
				},
			}
			Expect(servicePredicate(serviceInTestNs)).To(BeFalse())

			// Test with invalid object type - should return false
			Expect(deploymentPredicate("invalid-object")).To(BeFalse())
		})

		It("Should handle selective informer updates efficiently", func() {
			reconciler := &SinkFilterReconciler{
				informers: make(map[string]*InformerInfo),
				// cloudEventPublisher is nil in tests - controller handles this gracefully
			}

			// Simulate initial state with some existing informers
			reconciler.informers["Deployment-apps/v1"] = &InformerInfo{
				Factory: nil, // nil for test purposes
				StopCh:  make(chan struct{}),
			}
			reconciler.informers["Pod-v1"] = &InformerInfo{
				Factory: nil, // nil for test purposes
				StopCh:  make(chan struct{}),
			}

			// Test finding informers to stop
			newUniqueKinds := map[string]sourcesv1.APIVersionKindSelector{
				"Deployment-apps/v1": {Kind: "Deployment", APIVersion: "apps/v1"}, // Keep
				"Service-v1":         {Kind: "Service", APIVersion: "v1"},         // New
				// Pod-v1 is missing, so should be stopped
			}

			toStop := reconciler.findInformersToStop(newUniqueKinds)
			Expect(len(toStop)).To(Equal(1))
			Expect(toStop).To(ContainElement("Pod-v1"))

			// Test finding informers to create
			toCreate := reconciler.findInformersToCreate(newUniqueKinds)
			Expect(len(toCreate)).To(Equal(1))
			Expect(toCreate).To(ContainElement("Service-v1"))

			// Verify unchanged informers are identified correctly
			unchanged := 0
			for key := range reconciler.informers {
				if _, stillNeeded := newUniqueKinds[key]; stillNeeded {
					unchanged++
				}
			}
			Expect(unchanged).To(Equal(1)) // Only Deployment should remain unchanged
		})
	})
})

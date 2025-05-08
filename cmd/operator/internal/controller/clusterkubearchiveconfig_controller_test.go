// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

var _ = Describe("KubeArchiveConfig Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		installApiName := types.NamespacedName{
			Name:      constants.KubeArchiveApiServerSourceName,
			Namespace: constants.KubeArchiveNamespace,
		}
		installSFName := types.NamespacedName{
			Name:      constants.SinkFilterResourceName,
			Namespace: constants.KubeArchiveNamespace,
		}

		clusterApiName := types.NamespacedName{Name: constants.KubeArchiveApiServerSourceName}
		clusterKACName := types.NamespacedName{
			Name: constants.KubeArchiveConfigResourceName,
		}

		kacJobsPolicyRule := rbacv1.PolicyRule{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs"},
			Verbs:     []string{"get", "list", "watch"},
		}
		kacPodsPolicyRule := rbacv1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		}

		jobResource := kubearchivev1.KubeArchiveConfigResource{
			ArchiveWhen: "status.state != 'Completed'",
			DeleteWhen:  "status.state == 'Completed'",
			Selector: sourcesv1.APIVersionKindSelector{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
		}
		podResource := kubearchivev1.KubeArchiveConfigResource{
			ArchiveOnDelete: "true",
			Selector: sourcesv1.APIVersionKindSelector{
				APIVersion: "v1",
				Kind:       "Pod",
			},
		}
		ckac := &kubearchivev1.ClusterKubeArchiveConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.KubeArchiveConfigResourceName,
			},
			Spec: kubearchivev1.ClusterKubeArchiveConfigSpec{
				Resources: []kubearchivev1.KubeArchiveConfigResource{jobResource},
			},
		}

		kubearchiveconfig := &kubearchivev1.ClusterKubeArchiveConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterKubeArchiveConfig")
			Expect(k8sClient.Create(ctx, ckac)).To(Succeed())
		})

		AfterEach(func() {
			By("Confirm custom resource deletion")
			err := k8sClient.Get(ctx, clusterKACName, kubearchiveconfig)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			for _, op := range []string{"create", "add-resource", "remove-resource", "delete"} {
				By(op)
				if op == "update-add-resouce" {
					Expect(k8sClient.Get(ctx, clusterKACName, ckac)).To(Succeed())
					ckac.Spec.Resources = append(ckac.Spec.Resources, podResource)
					Expect(k8sClient.Update(ctx, ckac)).To(Succeed())
				} else if op == "update-remove-resource" {
					Expect(k8sClient.Get(ctx, clusterKACName, ckac)).To(Succeed())
					ckac.Spec.Resources = []kubearchivev1.KubeArchiveConfigResource{jobResource}
					Expect(k8sClient.Update(ctx, ckac)).To(Succeed())
				} else if op == "delete" {
					Expect(k8sClient.Delete(ctx, ckac)).To(Succeed())
				}
				controllerReconciler := &ClusterKubeArchiveConfigReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
					Mapper: k8sClient.RESTMapper(),
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: clusterKACName,
				})
				Expect(err).NotTo(HaveOccurred())

				// Check that SinkFilters exists and has the namespace in it.
				sf := &kubearchivev1.SinkFilter{}
				err = k8sClient.Get(ctx, installSFName, sf)
				Expect(err).NotTo(HaveOccurred())
				if op == "delete" {
					Expect(len(sf.Spec.Namespaces)).To(Equal(0))
				} else {
					Expect(len(sf.Spec.Namespaces)).To(Equal(1))
					Expect(sf.Spec.Namespaces).Should(HaveKey(constants.SinkFilterGlobalNamespace))
				}

				// Check for the ApiServerSource ServiceAccount in the kubearchive namespace.
				err = k8sClient.Get(ctx, installApiName, &corev1.ServiceAccount{})
				Expect(err).NotTo(HaveOccurred())

				// Check for the ApiServerSource cluster role.
				clusterRole := &rbacv1.ClusterRole{}
				err = k8sClient.Get(ctx, clusterApiName, clusterRole)
				Expect(err).NotTo(HaveOccurred())
				if op == "create" {
					Expect(len(clusterRole.Rules)).To(Equal(1))
					Expect(clusterRole.Rules).Should(ContainElement(kacJobsPolicyRule))
				} else if op == "update-add-resource" {
					Expect(len(clusterRole.Rules)).To(Equal(2))
					Expect(clusterRole.Rules).Should(ContainElement(kacJobsPolicyRule))
					Expect(clusterRole.Rules).Should(ContainElement(kacPodsPolicyRule))
				} else if op == "update-remove-resource" {
					Expect(len(clusterRole.Rules)).To(Equal(1))
					Expect(clusterRole.Rules).Should(ContainElement(kacJobsPolicyRule))
					Expect(clusterRole.Rules).Should(Not(ContainElement(kacPodsPolicyRule)))
				} else if op == "delete" {
					Expect(len(clusterRole.Rules)).To(Equal(0))
				}

				if op != "delete" {
					// Check for the ApiServerSource in the KubeArchive installation namespace.
					err = k8sClient.Get(ctx, installApiName, &sourcesv1.ApiServerSource{})
					Expect(err).NotTo(HaveOccurred())
				}
			}
		})
	})
})

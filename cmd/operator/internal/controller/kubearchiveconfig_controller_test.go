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
		const namespaceName = "kac-test"

		ctx := context.Background()

		nsName := types.NamespacedName{Name: namespaceName}

		localBrokerRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.KubeArchiveVacuumBroker,
		}
		localKACName := types.NamespacedName{
			Name:      constants.KubeArchiveConfigResourceName,
			Namespace: namespaceName,
		}
		localVacName := types.NamespacedName{
			Name:      constants.KubeArchiveVacuumName,
			Namespace: namespaceName,
		}
		localSinkName := types.NamespacedName{
			Name:      constants.KubeArchiveSinkName,
			Namespace: namespaceName,
		}
		localApiName := types.NamespacedName{
			Name:      constants.KubeArchiveApiServerSourceName,
			Namespace: namespaceName,
		}
		installApiName := types.NamespacedName{
			Name:      constants.KubeArchiveApiServerSourceName,
			Namespace: constants.KubeArchiveNamespace,
		}
		installBrokerName := types.NamespacedName{
			Name:      constants.KubeArchiveVacuumBroker,
			Namespace: constants.KubeArchiveNamespace,
		}
		installSFName := types.NamespacedName{
			Name:      constants.SinkFilterResourceName,
			Namespace: constants.KubeArchiveNamespace,
		}

		localVacSA := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      constants.KubeArchiveVacuumName,
			Namespace: namespaceName,
		}
		installApiSA := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      constants.KubeArchiveApiServerSourceName,
			Namespace: constants.KubeArchiveNamespace,
		}
		installClusterVacSA := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      constants.KubeArchiveClusterVacuumName,
			Namespace: constants.KubeArchiveNamespace,
		}
		installSinkSA := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      constants.KubeArchiveSinkName,
			Namespace: constants.KubeArchiveNamespace,
		}

		apiRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     constants.KubeArchiveApiServerSourceName,
		}
		ckacRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     constants.ClusterKubeArchiveConfigClusterRoleBindingName,
		}
		sinkRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.KubeArchiveSinkName,
		}
		vacRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.KubeArchiveVacuumName,
		}

		clusterApiName := types.NamespacedName{Name: constants.KubeArchiveApiServerSourceName}
		clusterCKACName := types.NamespacedName{Name: constants.ClusterKubeArchiveConfigClusterRoleBindingName}

		brokerPolicyRule := rbacv1.PolicyRule{
			APIGroups: []string{"eventing.knative.dev"},
			Resources: []string{"brokers"},
			Verbs:     []string{"get", "list"},
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
		sinkJobsPolicyRule := rbacv1.PolicyRule{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs"},
			Verbs:     []string{"delete"},
		}
		sinkPodsPolicyRule := rbacv1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"delete"},
		}
		vacPolicyRule := rbacv1.PolicyRule{
			APIGroups: []string{"kubearchive.org"},
			Resources: []string{"kubearchiveconfigs", "namespacevacuumconfigs"},
			Verbs:     []string{"get", "list"},
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
		kac := &kubearchivev1.KubeArchiveConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.KubeArchiveConfigResourceName,
				Namespace: namespaceName,
			},
			Spec: kubearchivev1.KubeArchiveConfigSpec{
				Resources: []kubearchivev1.KubeArchiveConfigResource{jobResource},
			},
		}
		ckac := &kubearchivev1.ClusterKubeArchiveConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.KubeArchiveConfigResourceName,
			},
			Spec: kubearchivev1.ClusterKubeArchiveConfigSpec{
				Resources: []kubearchivev1.KubeArchiveConfigResource{podResource},
			},
		}
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}

		kubearchiveconfig := &kubearchivev1.KubeArchiveConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind KubeArchiveConfig")
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			Expect(k8sClient.Create(ctx, kac)).To(Succeed())
		})

		AfterEach(func() {
			By("Confirm custom resource deletion")
			err := k8sClient.Get(ctx, localKACName, kubearchiveconfig)
			Expect(errors.IsNotFound(err)).To(BeTrue())
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			for _, op := range []string{"create", "update-add-resouce", "update-remove-resource", "ckac-add-resource", "ckac-remove-resource", "delete"} {
				By(op)
				if op == "update-add-resouce" {
					Expect(k8sClient.Get(ctx, localKACName, kac)).To(Succeed())
					kac.Spec.Resources = append(kac.Spec.Resources, podResource)
					Expect(k8sClient.Update(ctx, kac)).To(Succeed())
				} else if op == "update-remove-resource" {
					Expect(k8sClient.Get(ctx, localKACName, kac)).To(Succeed())
					kac.Spec.Resources = []kubearchivev1.KubeArchiveConfigResource{jobResource}
					Expect(k8sClient.Update(ctx, kac)).To(Succeed())
				} else if op == "ckac-add-resource" {
					Expect(k8sClient.Create(ctx, ckac)).To(Succeed())
				} else if op == "ckac-remove-resource" {
					Expect(k8sClient.Delete(ctx, ckac)).To(Succeed())
				} else if op == "delete" {
					Expect(k8sClient.Delete(ctx, kac)).To(Succeed())
				}
				controllerReconciler := &KubeArchiveConfigReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					Mapper:     k8sClient.RESTMapper(),
					UseKnative: true,
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: localKACName,
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
					Expect(sf.Spec.Namespaces).Should(HaveKey(namespaceName))
				}

				// Check for the ApiServerSource Service account in the kubearchive namespace.
				err = k8sClient.Get(ctx, installApiName, &corev1.ServiceAccount{})
				Expect(err).NotTo(HaveOccurred())

				// Check for the ApiServerSource cluster role.
				clusterRole := &rbacv1.ClusterRole{}
				err = k8sClient.Get(ctx, clusterApiName, clusterRole)
				Expect(err).NotTo(HaveOccurred())
				if op == "create" || op == "ckac-add-resource" || op == "ckac-remove-resource" {
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

					// Check for the ApiServerSource Service role binding in the namespace.
					roleBinding := &rbacv1.RoleBinding{}
					err = k8sClient.Get(ctx, localApiName, roleBinding)
					Expect(err).NotTo(HaveOccurred())
					Expect(roleBinding.RoleRef).To(Equal(apiRoleRef))
					Expect(roleBinding.Subjects).Should(ContainElement(installApiSA))

					// Check for the sink role in the namespace.
					role := &rbacv1.Role{}
					err = k8sClient.Get(ctx, localSinkName, role)
					Expect(err).NotTo(HaveOccurred())
					Expect(role.Rules).Should(ContainElement(sinkJobsPolicyRule))
					if op == "update-add-resource" || op == "ckac-add-resource" {
						Expect(len(role.Rules)).To(Equal(2))
						Expect(role.Rules).Should(ContainElement(sinkPodsPolicyRule))
					} else if op == "update-remove-resource" || op == "ckac-remove-resource" {
						Expect(len(role.Rules)).To(Equal(1))
						Expect(role.Rules).Should(Not(ContainElement(sinkPodsPolicyRule)))
					}

					// Check for the sink role binding in the namespace.
					roleBinding = &rbacv1.RoleBinding{}
					err = k8sClient.Get(ctx, localSinkName, roleBinding)
					Expect(err).NotTo(HaveOccurred())
					Expect(roleBinding.RoleRef).To(Equal(sinkRoleRef))
					Expect(roleBinding.Subjects).Should(ContainElement(installSinkSA))
				}

				// Check the namespace.
				ns := &corev1.Namespace{}
				err = k8sClient.Get(ctx, nsName, ns)
				Expect(err).NotTo(HaveOccurred())
				if op != "delete" {
					Expect(ns.ObjectMeta.Labels).Should(HaveKeyWithValue(ApiServerSourceLabelName, "true"))
				} else {
					Expect(ns.ObjectMeta.Labels).Should(Not(HaveKey(ApiServerSourceLabelName)))
				}

				if op != "delete" {
					// Check the vacuum service account in the namespace.
					err = k8sClient.Get(ctx, localVacName, &corev1.ServiceAccount{})
					Expect(err).NotTo(HaveOccurred())

					// Check for the vacuum role in the namespace.
					role := &rbacv1.Role{}
					err = k8sClient.Get(ctx, localVacName, role)
					Expect(err).NotTo(HaveOccurred())

					Expect(role.Rules).Should(ContainElement(vacPolicyRule))

					// Check for the vacuum role binding in the namespace.
					roleBinding := &rbacv1.RoleBinding{}
					err = k8sClient.Get(ctx, localVacName, roleBinding)
					Expect(err).NotTo(HaveOccurred())
					Expect(roleBinding.RoleRef).To(Equal(vacRoleRef))
					Expect(roleBinding.Subjects).Should(ContainElement(localVacSA))
					Expect(roleBinding.Subjects).Should(ContainElement(installClusterVacSA))

					// Check for the vacuum broker role in the kubearchive namespace.
					role = &rbacv1.Role{}
					err = k8sClient.Get(ctx, installBrokerName, role)
					Expect(err).NotTo(HaveOccurred())
					Expect(role.Rules).Should(ContainElement(brokerPolicyRule))
				}

				// Check for the vacuum broker role binding in the kubearchive namespace.
				roleBinding := &rbacv1.RoleBinding{}
				err = k8sClient.Get(ctx, installBrokerName, roleBinding)
				Expect(err).NotTo(HaveOccurred())
				Expect(roleBinding.RoleRef).To(Equal(localBrokerRoleRef))
				if op != "delete" {
					Expect(roleBinding.Subjects).Should(ContainElement(localVacSA))
				} else {
					Expect(roleBinding.Subjects).Should(Not(ContainElement(localVacSA)))
				}
				Expect(roleBinding.Subjects).Should(ContainElement(installClusterVacSA))

				// Check for the ClusterKubeArchiveConfig cluster role binding.
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				err = k8sClient.Get(ctx, clusterCKACName, clusterRoleBinding)
				Expect(err).NotTo(HaveOccurred())
				Expect(clusterRoleBinding.RoleRef).To(Equal(ckacRoleRef))
				if op != "delete" {
					Expect(clusterRoleBinding.Subjects).Should(ContainElement(localVacSA))
				} else {
					Expect(clusterRoleBinding.Subjects).Should(Not(ContainElement(localVacSA)))
				}
				Expect(clusterRoleBinding.Subjects).Should(ContainElement(installClusterVacSA))
			}
		})
	})
})

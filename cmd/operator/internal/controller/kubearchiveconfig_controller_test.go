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
)

var _ = Describe("KubeArchiveConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const namespaceName = "kac-test"

		ctx := context.Background()

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
		installSFName := types.NamespacedName{
			Name:      constants.SinkFilterResourceName,
			Namespace: constants.KubeArchiveNamespace,
		}
		installRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.KubeArchiveVacuumName,
		}
		installRoleBindingName := types.NamespacedName{
			Name:      constants.KubeArchiveVacuumName,
			Namespace: constants.KubeArchiveNamespace,
		}

		localVacSA := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      constants.KubeArchiveVacuumName,
			Namespace: namespaceName,
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
			Resources: []string{"namespacevacuumconfigs"},
			Verbs:     []string{"get", "list"},
		}

		jobResource := kubearchivev1.KubeArchiveConfigResource{
			ArchiveWhen: "status.state != 'Completed'",
			DeleteWhen:  "status.state == 'Completed'",
			Selector: kubearchivev1.APIVersionKind{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
		}
		podResource := kubearchivev1.KubeArchiveConfigResource{
			ArchiveOnDelete: "true",
			Selector: kubearchivev1.APIVersionKind{
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
		It("should successfully reconcile the KubeArchiveConfig resource", func() {
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
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
					Mapper: k8sClient.RESTMapper(),
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

				if op != "delete" {
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
					roleBinding := &rbacv1.RoleBinding{}
					err = k8sClient.Get(ctx, localSinkName, roleBinding)
					Expect(err).NotTo(HaveOccurred())
					Expect(roleBinding.RoleRef).To(Equal(sinkRoleRef))
					Expect(roleBinding.Subjects).Should(ContainElement(installSinkSA))
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
				}

				// Check for the vacuum role binding in the kubearchive namespace.
				roleBinding := &rbacv1.RoleBinding{}
				err = k8sClient.Get(ctx, installRoleBindingName, roleBinding)
				Expect(err).NotTo(HaveOccurred())
				Expect(roleBinding.RoleRef).To(Equal(installRoleRef))
				if op != "delete" {
					Expect(roleBinding.Subjects).Should(ContainElement(localVacSA))
				} else {
					Expect(roleBinding.Subjects).Should(Not(ContainElement(localVacSA)))
				}
				Expect(roleBinding.Subjects).Should(ContainElement(installClusterVacSA))

			}
		})
	})
})

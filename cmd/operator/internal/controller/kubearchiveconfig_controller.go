// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a13e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

// KubeArchiveConfigReconciler reconciles a KubeArchiveConfig object
type KubeArchiveConfigReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Mapper meta.RESTMapper
}

//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=sinkfilters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
//+kubebuilder:rbac:groups=sources.knative.dev,resources=apiserversources,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;update;watch

func (r *KubeArchiveConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Reconciling KubeArchiveConfig")

	kaconfig := &kubearchivev1alpha1.KubeArchiveConfig{}
	if err := r.Client.Get(ctx, req.NamespacedName, kaconfig); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue (we need
		// to wait for a new notification), and we can get them on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if kaconfig.DeletionTimestamp.IsZero() {
		// The object is not being deleted, add the finalizer if necessary.
		if !controllerutil.ContainsFinalizer(kaconfig, resourceFinalizerName) {
			controllerutil.AddFinalizer(kaconfig, resourceFinalizerName)
			if err := r.Client.Update(ctx, kaconfig); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted.
		if controllerutil.ContainsFinalizer(kaconfig, resourceFinalizerName) {
			// Finalizer is present, clean up filters from ConfigMap and remove Namespace label.

			log.Info("Deleting KubeArchiveConfig")

			if _, err := reconcileAllCommonResources(ctx, r.Client, r.Mapper, kaconfig.Namespace, nil); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.removeNamespaceLabel(ctx, kaconfig); err != nil {
				// If label removal fails, return with error so that it can be retried.
				return ctrl.Result{}, err
			}

			// Remove the finalizer from the list and update it.
			controllerutil.RemoveFinalizer(kaconfig, resourceFinalizerName)
			if err := r.Client.Update(ctx, kaconfig); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the resource is being deleted.
		return ctrl.Result{}, nil
	}

	var err error
	var clusterrole *rbacv1.ClusterRole
	if clusterrole, err = reconcileAllCommonResources(ctx, r.Client, r.Mapper, kaconfig.Namespace, kaconfig.Spec.Resources); err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileA13eRoleBinding(ctx, kaconfig, clusterrole)
	if err != nil {
		return ctrl.Result{}, err
	}

	role, err := r.reconcileSinkRole(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileSinkRoleBinding(ctx, kaconfig, role)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileNamespace(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeArchiveConfigReconciler) SetupKubeArchiveConfigWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1alpha1.KubeArchiveConfig{}).
		//Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	resources := make([]sourcesv1.APIVersionKindSelector, 0)
	for _, kar := range kaconfig.Spec.Resources {
		resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
		resources = append(resources, resource)
	}
	return r.reconcileRole(ctx, kaconfig, constants.KubeArchiveSinkName, createPolicyRules(ctx, r.Mapper, resources, []string{"delete"}))
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, name string, rules []rbacv1.PolicyRule) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRole " + name)
	desired, err := r.desiredRole(kaconfig, name, rules)
	if err != nil {
		log.Error(err, "Unable to get desired Role "+name)
		return nil, err
	}

	existing := &rbacv1.Role{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: kaconfig.Namespace}, existing)
	if errors.IsNotFound(err) {
		err = r.Client.Create(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to create Role "+name)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile Role "+name)
		return nil, err
	}

	existing.Rules = desired.Rules
	err = r.Client.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update Role "+name)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) desiredRole(kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string, rules []rbacv1.PolicyRule) (*rbacv1.Role, error) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: kaconfig.Namespace,
		},
		Rules: rules,
	}

	if err := ctrl.SetControllerReference(kaconfig, role, r.Scheme); err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileA13eRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, role *rbacv1.ClusterRole) (*rbacv1.RoleBinding, error) {
	return r.reconcileRoleBinding(ctx, kaconfig, role.Name, "ClusterRole")
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {
	return r.reconcileRoleBinding(ctx, kaconfig, role.Name, "Role")
}

func (r *KubeArchiveConfigReconciler) reconcileRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, name string, kind string) (*rbacv1.RoleBinding, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRoleBinding " + name)
	desired, err := r.desiredRoleBinding(kaconfig, name, kind)
	if err != nil {
		log.Error(err, "Unable to get desired RoleBinding "+name)
		return nil, err
	}

	existing := &rbacv1.RoleBinding{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: kaconfig.Namespace}, existing)
	if errors.IsNotFound(err) {
		err = r.Client.Create(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to create RoleBinding "+name)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile RoleBinding "+name)
		return nil, err
	}

	existing.RoleRef = desired.RoleRef
	existing.Subjects = desired.Subjects
	err = r.Client.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update RoleBinding "+name)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) desiredRoleBinding(kaconfig *kubearchivev1alpha1.KubeArchiveConfig, name string, kind string) (*rbacv1.RoleBinding, error) {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: kaconfig.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     kind,
			Name:     name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      name,
			Namespace: constants.KubeArchiveNamespace,
		}},
	}

	if err := ctrl.SetControllerReference(kaconfig, binding, r.Scheme); err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileNamespace(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.Namespace, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileNamespace")

	ns := &corev1.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: kaconfig.Namespace}, ns)
	if err != nil {
		log.Error(err, "Failed to get Namespace "+kaconfig.Namespace)
		return nil, err
	}

	ns.Labels[ApiServerSourceLabelName] = ApiServerSourceLabelValue
	err = r.Client.Update(ctx, ns)
	if err != nil {
		log.Error(err, "Failed to update Namespace "+kaconfig.Namespace)
		return nil, err
	}

	return ns, nil
}

func (r *KubeArchiveConfigReconciler) removeNamespaceLabel(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) error {
	log := log.FromContext(ctx)

	log.Info("in removeNamespaceLabel")

	ns := &corev1.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: kaconfig.Namespace}, ns)
	if err != nil {
		log.Error(err, "Failed to get Namespace "+kaconfig.Namespace)
		return err
	}

	delete(ns.Labels, ApiServerSourceLabelName)

	err = r.Client.Update(ctx, ns)
	if err != nil {
		log.Error(err, "Failed to update Namespace "+kaconfig.Namespace)
		return err
	}

	return nil
}

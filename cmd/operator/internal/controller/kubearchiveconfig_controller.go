// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
)

// KubeArchiveConfigReconciler reconciles a KubeArchiveConfig object
type KubeArchiveConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
//+kubebuilder:rbac:groups=sources.knative.dev,resources=apiserversources,verbs=create;delete;get;list;update;watch

func (r *KubeArchiveConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("KubeArchiveConfig reconciling.")

	kaconfig := &kubearchivev1alpha1.KubeArchiveConfig{}
	err := r.Get(ctx, req.NamespacedName, kaconfig)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("KubeArchiveConfig resource not found. Ignoring since object must have been deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get KubeArchiveConfig, requeuing the request.")
		return ctrl.Result{}, err
	}

	r.reconcileServiceAccount(ctx, kaconfig)
	r.reconcileRole(ctx, kaconfig)
	r.reconcileRoleBinding(ctx, kaconfig)
	r.reconcileApiServerSource(ctx, kaconfig)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeArchiveConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1alpha1.KubeArchiveConfig{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&sourcesv1.ApiServerSource{}).
		Complete(r)
}

func (r *KubeArchiveConfigReconciler) reconcileServiceAccount(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileServiceAccount")
	sa, err := r.desiredServiceAccount(kaconfig)
	if err != nil {
		log.Error(err, "Unable to get desired ServiceAccount")
		return sa, err
	}

	err = r.Get(ctx, types.NamespacedName{Name: kaconfig.Name, Namespace: kaconfig.Namespace}, &corev1.ServiceAccount{})
	if err == nil {
		err = r.Update(ctx, sa)
		if err != nil {
			log.Error(err, "Failed to update ServiceAccount")
			return sa, err
		}
	} else if errors.IsNotFound(err) {
		err = r.Create(ctx, sa)
		if err != nil {
			log.Error(err, "Failed to create ServiceAccount")
			return sa, err
		}
	} else {
		log.Error(err, "Failed to reconcile ServiceAccount")
		return sa, err
	}

	return sa, nil
}

func (r *KubeArchiveConfigReconciler) desiredServiceAccount(kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaconfig.Name,
			Namespace: kaconfig.Namespace,
		},
	}

	if err := ctrl.SetControllerReference(kaconfig, sa, r.Scheme); err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRole")
	role, err := r.desiredRole(kaconfig)
	if err != nil {
		log.Error(err, "Unable to get desired Role")
		return role, err
	}

	err = r.Get(ctx, types.NamespacedName{Name: kaconfig.Name, Namespace: kaconfig.Namespace}, &rbacv1.Role{})
	if err == nil {
		err = r.Update(ctx, role)
		if err != nil {
			log.Error(err, "Failed to update Role")
			return role, err
		}
	} else if errors.IsNotFound(err) {
		err = r.Create(ctx, role)
		if err != nil {
			log.Error(err, "Failed to create Role")
			return role, err
		}
	} else {
		log.Error(err, "Failed to reconcile Role")
		return role, err
	}

	return role, nil
}

func (r *KubeArchiveConfigReconciler) desiredRole(kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	var rules []rbacv1.PolicyRule
	for _, resource := range kaconfig.Spec.Resources {
		rules = append(rules, rbacv1.PolicyRule{APIGroups: []string{resource.APIVersion}, Resources: []string{resource.Kind}, Verbs: []string{"get", "list", "watch"}})
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaconfig.Name,
			Namespace: kaconfig.Namespace,
		},
		Rules: rules,
	}

	if err := ctrl.SetControllerReference(kaconfig, role, r.Scheme); err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.RoleBinding, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRoleBinding")
	binding, err := r.desiredRoleBinding(kaconfig)
	if err != nil {
		log.Error(err, "Unable to get desired RoleBinding")
		return binding, err
	}

	err = r.Get(ctx, types.NamespacedName{Name: kaconfig.Name, Namespace: kaconfig.Namespace}, &rbacv1.RoleBinding{})
	if err == nil {
		err = r.Update(ctx, binding)
		if err != nil {
			log.Error(err, "Failed to update RoleBinding")
			return binding, err
		}
	} else if errors.IsNotFound(err) {
		err = r.Create(ctx, binding)
		if err != nil {
			log.Error(err, "Failed to create RoleBinding")
			return binding, err
		}
	} else {
		log.Error(err, "Failed to reconcile RoleBinding")
		return binding, err
	}

	return binding, nil
}

func (r *KubeArchiveConfigReconciler) desiredRoleBinding(kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.RoleBinding, error) {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaconfig.Name,
			Namespace: kaconfig.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     kaconfig.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      kaconfig.Name,
			Namespace: kaconfig.Namespace,
		}},
	}

	if err := ctrl.SetControllerReference(kaconfig, binding, r.Scheme); err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileApiServerSource(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*sourcesv1.ApiServerSource, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileApiServerSource")
	source, err := r.desiredApiServerSource(kaconfig)
	if err != nil {
		log.Error(err, "Unable to get desired ApiServerSource")
		return source, err
	}

	existing := &sourcesv1.ApiServerSource{}
	err = r.Get(ctx, types.NamespacedName{Name: kaconfig.Name, Namespace: kaconfig.Namespace}, existing)
	if err == nil {
		source.SetResourceVersion(existing.GetResourceVersion())
		err = r.Update(ctx, source)
		if err != nil {
			log.Error(err, "Failed to update ApiServerSource")
			return source, err
		}
	} else if errors.IsNotFound(err) {
		err = r.Create(ctx, source)
		if err != nil {
			log.Error(err, "Failed to create ApiServerSource")
			return source, err
		}
	} else {
		log.Error(err, "Failed to reconcile ApiServerSource")
		return source, err
	}

	return source, nil
}

func (r *KubeArchiveConfigReconciler) desiredApiServerSource(kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*sourcesv1.ApiServerSource, error) {
	source := &sourcesv1.ApiServerSource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiServerSource",
			APIVersion: "sources.knative.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaconfig.Name,
			Namespace: kaconfig.Namespace,
		},
		Spec: sourcesv1.ApiServerSourceSpec{
			EventMode:          "Resource",
			ServiceAccountName: kaconfig.Name,
			Resources:          kaconfig.Spec.Resources,
			SourceSpec: duckv1.SourceSpec{
				Sink: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: "v1",
						Kind:       "Service",
						Name:       "kubearchive-sink",
						Namespace:  "kubearchive",
					},
				},
			},
		},
	}

	return source, nil
}

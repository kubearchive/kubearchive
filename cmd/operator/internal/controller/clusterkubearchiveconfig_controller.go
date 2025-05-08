// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a13e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"

	meta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

// ClusterKubeArchiveConfigReconciler reconciles a ClusterKubeArchiveConfig object
type ClusterKubeArchiveConfigReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Mapper meta.RESTMapper
}

//+kubebuilder:rbac:groups=kubearchive.org,resources=clusterkubearchiveconfigs;clustervacuums;namespacevacuums;sinkfilters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.org,resources=clusterkubearchiveconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubearchive.org,resources=clusterkubearchiveconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;clusterroles;roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
//+kubebuilder:rbac:groups=sources.knative.dev,resources=apiserversources,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;update;watch

func (r *ClusterKubeArchiveConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Reconciling ClusterKubeArchiveConfig")

	ckaconfig := &kubearchivev1.ClusterKubeArchiveConfig{}
	if err := r.Client.Get(ctx, req.NamespacedName, ckaconfig); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue (we need
		// to wait for a new notification), and we can get them on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if ckaconfig.DeletionTimestamp.IsZero() {
		// The object is not being deleted, add the finalizer if necessary.
		if !controllerutil.ContainsFinalizer(ckaconfig, resourceFinalizerName) {
			controllerutil.AddFinalizer(ckaconfig, resourceFinalizerName)
			if err := r.Client.Update(ctx, ckaconfig); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted.
		if controllerutil.ContainsFinalizer(ckaconfig, resourceFinalizerName) {
			// Finalizer is present, reconcile all with resources set to nil

			log.Info("Deleting ClusterKubeArchiveConfig")

			if _, err := reconcileAllCommonResources(ctx, r.Client, r.Mapper, constants.SinkFilterGlobalNamespace, nil); err != nil {
				return ctrl.Result{}, err
			}

			// Remove the finalizer from the list and update it.
			controllerutil.RemoveFinalizer(ckaconfig, resourceFinalizerName)
			if err := r.Client.Update(ctx, ckaconfig); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the resource is being deleted.
		return ctrl.Result{}, nil
	}

	if _, err := reconcileAllCommonResources(ctx, r.Client, r.Mapper, constants.SinkFilterGlobalNamespace, ckaconfig.Spec.Resources); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterKubeArchiveConfigReconciler) SetupClusterKubeArchiveConfigWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1.ClusterKubeArchiveConfig{}).
		//Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

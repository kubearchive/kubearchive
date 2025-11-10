// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

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

			if err := updateSinkFilterCluster(ctx, r.Client, nil); err != nil {
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

	if err := updateSinkFilterCluster(ctx, r.Client, ckaconfig.Spec.Resources); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterKubeArchiveConfigReconciler) SetupClusterKubeArchiveConfigWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1.ClusterKubeArchiveConfig{}).
		//Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func updateSinkFilterCluster(ctx context.Context, client client.Client, resources []kubearchivev1.KubeArchiveConfigResource) error {
	log := log.FromContext(ctx)

	log.Info("in updateSinkFilterCluster")

	sf := &kubearchivev1.SinkFilter{}
	err := client.Get(ctx, types.NamespacedName{Name: constants.SinkFilterResourceName, Namespace: constants.KubeArchiveNamespace}, sf)
	if errors.IsNotFound(err) {
		sf = desiredSinkFilterCluster(ctx, nil, resources)
		err = client.Create(ctx, sf)
		if err != nil {
			log.Error(err, "Failed to create SinkFilter "+constants.SinkFilterResourceName)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile SinkFilter "+constants.SinkFilterResourceName)
		return err
	}

	sf = desiredSinkFilterCluster(ctx, sf, resources)
	err = client.Update(ctx, sf)
	if err != nil {
		log.Error(err, "Failed to update SinkFilter "+constants.SinkFilterResourceName)
		return err
	}
	return nil
}

func desiredSinkFilterCluster(ctx context.Context, sf *kubearchivev1.SinkFilter, resources []kubearchivev1.KubeArchiveConfigResource) *kubearchivev1.SinkFilter {
	log := log.FromContext(ctx)

	log.Info("in desiredSinkFilterCluster")

	if sf == nil {
		sf = &kubearchivev1.SinkFilter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.SinkFilterResourceName,
				Namespace: constants.KubeArchiveNamespace,
			},
			Spec: kubearchivev1.SinkFilterSpec{
				Namespaces: map[string][]kubearchivev1.KubeArchiveConfigResource{},
			},
		}
	}

	if sf.Spec.Namespaces == nil {
		sf.Spec.Namespaces = make(map[string][]kubearchivev1.KubeArchiveConfigResource)
	}

	// Clean up older ClusterKubeArchiveConfig CEL expressions that were stored in a global namespace,
	// since they are now stored in the Cluster field of the SinkFilter object.
	delete(sf.Spec.Namespaces, constants.SinkFilterGlobalNamespace)

	if len(resources) > 0 {
		sf.Spec.Cluster = resources
	} else {
		sf.Spec.Cluster = []kubearchivev1.KubeArchiveConfigResource{}
	}

	// Note that the owner reference is NOT set on the SinkFilter resource.  It should not be deleted when
	// the KubeArchiveConfig object is deleted.
	return sf
}

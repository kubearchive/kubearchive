// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a13e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

// KubeArchiveConfigReconciler reconciles a KubeArchiveConfig object
type KubeArchiveConfigReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Mapper meta.RESTMapper
}

//+kubebuilder:rbac:groups=kubearchive.org,resources=clustervacuums;kubearchiveconfigs;namespacevacuums;sinkfilters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;clusterroles;roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;update;watch

func (r *KubeArchiveConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	slog.Info("Reconciling KubeArchiveConfig")

	kaconfig := &kubearchivev1.KubeArchiveConfig{}
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

			slog.Info("Deleting KubeArchiveConfig")

			if err := updateSinkFilterNamespace(ctx, r.Client, kaconfig.Namespace, nil); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.reconcileKubeArchiveVacuumRoleBinding(ctx, kaconfig, false); err != nil {
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

	if err := updateSinkFilterNamespace(ctx, r.Client, kaconfig.Namespace, kaconfig.Spec.Resources); err != nil {
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

	err = r.reconcileVacuumResources(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeArchiveConfigReconciler) SetupKubeArchiveConfigWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1.KubeArchiveConfig{}).
		//Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&kubearchivev1.ClusterKubeArchiveConfig{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, cm client.Object) []ctrl.Request {
			crList := &kubearchivev1.KubeArchiveConfigList{}
			if err := mgr.GetClient().List(ctx, crList); err != nil {
				mgr.GetLogger().Error(err, "while listing ExampleCRDWithConfigMapRefs")
				return nil
			}

			reqs := make([]ctrl.Request, 0, len(crList.Items))
			for _, item := range crList.Items {
				reqs = append(reqs, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: item.GetNamespace(),
						Name:      item.GetName(),
					},
				})
			}

			return reqs
		})).
		Complete(r)
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRole(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) (*rbacv1.Role, error) {

	resources := make([]kubearchivev1.APIVersionKind, 0)
	for _, kar := range kaconfig.Spec.Resources {
		resource := kubearchivev1.APIVersionKind{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
		resources = append(resources, resource)
	}

	ckac := &kubearchivev1.ClusterKubeArchiveConfig{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.KubeArchiveConfigResourceName}, ckac)
	if err == nil {
		for _, kar := range ckac.Spec.Resources {
			resource := kubearchivev1.APIVersionKind{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
			resources = append(resources, resource)
		}
	} else if !errors.IsNotFound(err) {
		slog.Error("Unable to get ClusterKubeArchiveConfig when reconciling sink role", "error", err)
	}

	role, err := r.reconcileRole(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveSinkName, createPolicyRules(ctx, r.Mapper, resources, []string{"delete"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, namespace string, name string, rules []rbacv1.PolicyRule) (*rbacv1.Role, error) {

	slog.Info("in reconcileRole", "name", name)
	desired := r.desiredRole(namespace, name, rules)
	if kaconfig != nil {
		if err := ctrl.SetControllerReference(kaconfig, desired, r.Scheme); err != nil {
			return nil, err
		}
	}

	existing := &rbacv1.Role{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		err = r.Client.Create(ctx, desired)
		if err != nil {
			slog.Error("Failed to create Role", "error", err, "name", name)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		slog.Error("Failed to reconcile Role", "error", err, "name", name)
		return nil, err
	}

	existing.Rules = desired.Rules
	err = r.Client.Update(ctx, existing)
	if err != nil {
		slog.Error("Failed to update Role", "error", err, "name", name)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) desiredRole(namespace string, name string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: rules,
	}
	return role
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {
	subject := newSubject(constants.KubeArchiveNamespace, role.Name)
	binding, err := r.reconcileRoleBinding(ctx, kaconfig, kaconfig.Namespace, role.Name, "Role", true, subject)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, namespace string, name string, kind string, add bool, subjects ...rbacv1.Subject) (*rbacv1.RoleBinding, error) {

	slog.Info("in reconcileRoleBinding", "name", name)
	desired := r.desiredRoleBinding(namespace, name, kind, subjects...)
	if kaconfig != nil {
		if err := ctrl.SetControllerReference(kaconfig, desired, r.Scheme); err != nil {
			return nil, err
		}
	}

	existing := &rbacv1.RoleBinding{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		err = r.Client.Create(ctx, desired)
		if err != nil {
			slog.Error("Failed to create RoleBinding", "error", err, "name", name)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		slog.Error("Failed to reconcile RoleBinding", "error", err, "name", name)
		return nil, err
	}

	if add {
		existing.Subjects = mergeSubjects(existing.Subjects, desired.Subjects)
	} else {
		existing.Subjects = removeSubjects(existing.Subjects, desired.Subjects...)
	}
	err = r.Client.Update(ctx, existing)
	if err != nil {
		slog.Error("Failed to update RoleBinding", "error", err, "name", name)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) desiredRoleBinding(namespace string, name string, kind string, subjects ...rbacv1.Subject) *rbacv1.RoleBinding {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     kind,
			Name:     name,
		},
		Subjects: subjects,
	}
	return binding
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumResources(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) error {
	if _, err := r.reconcileVacuumServiceAccount(ctx, kaconfig); err != nil {
		return err
	}

	role, err := r.reconcileVacuumRole(ctx, kaconfig)
	if err != nil {
		return err
	}

	if _, err := r.reconcileLocalVacuumRoleBinding(ctx, kaconfig, role); err != nil {
		return err
	}

	if err := r.reconcileKubeArchiveVacuumRoleBinding(ctx, kaconfig, true); err != nil {
		return err
	}

	return nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumServiceAccount(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {

	slog.Info("in reconcileVacuumServiceAccount")

	sa, err := r.reconcileServiceAccount(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveVacuumName)
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileServiceAccount(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, namespace string, name string) (*corev1.ServiceAccount, error) {

	slog.Info("in reconcileServiceAccount")
	sa := &corev1.ServiceAccount{}

	err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sa)
	if errors.IsNotFound(err) {
		sa = desiredServiceAccount(namespace, name)
		if kaconfig != nil {
			if err = ctrl.SetControllerReference(kaconfig, sa, r.Scheme); err != nil {
				return nil, err
			}
		}
		err = r.Client.Create(ctx, sa)
		if err != nil {
			slog.Error("Failed to create ServiceAccount", "error", err)
			return nil, err
		}
	} else if err != nil {
		slog.Error("Failed to reconcile ServiceAccount", "error", err)
		return nil, err
	}

	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumRole(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) (*rbacv1.Role, error) {

	slog.Info("in reconcileVacuumRole")

	resources := []kubearchivev1.APIVersionKind{
		{
			APIVersion: "kubearchive.org/v1",
			Kind:       "NamespaceVacuumConfig",
		},
	}

	for _, kar := range kaconfig.Spec.Resources {
		resource := kubearchivev1.APIVersionKind{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
		resources = append(resources, resource)
	}

	ckac := &kubearchivev1.ClusterKubeArchiveConfig{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.KubeArchiveConfigResourceName}, ckac)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
	} else {
		for _, kar := range ckac.Spec.Resources {
			resource := kubearchivev1.APIVersionKind{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
			resources = append(resources, resource)
		}
	}

	role, err := r.reconcileRole(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveVacuumName, createPolicyRules(ctx, r.Mapper, resources, []string{"get", "list"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

// Reconcile the vacuum role binding in the KubeArchiveConfig namespace.
func (r *KubeArchiveConfigReconciler) reconcileLocalVacuumRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {

	slog.Info("in reconcileLocalVacuumRoleBinding")

	ns := newSubject(kaconfig.Namespace, constants.KubeArchiveVacuumName)
	ka := newSubject(constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName)
	binding, err := r.reconcileRoleBinding(ctx, kaconfig, kaconfig.Namespace, role.Name, "Role", true, ns, ka)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

// Reconcile the vacuum role binding in the kubearchive namespace.
func (r *KubeArchiveConfigReconciler) reconcileKubeArchiveVacuumRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, add bool) error {

	slog.Info("in reconcileKubeArchiveVacuumRoleBinding")

	subjects := []rbacv1.Subject{newSubject(kaconfig.Namespace, constants.KubeArchiveVacuumName)}
	// Don't ever remove the cluster vacuum SA, but always make sure it is there.
	if add {
		subjects = append(subjects, newSubject(constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName))
	}
	_, err := r.reconcileRoleBinding(ctx, nil, constants.KubeArchiveNamespace, constants.KubeArchiveVacuumName, "Role", add, subjects...)
	if err != nil {
		return err
	}
	return nil
}

func updateSinkFilterNamespace(ctx context.Context, client client.Client, namespace string, resources []kubearchivev1.KubeArchiveConfigResource) error {

	slog.Info("in updateSinkFilterNamespace")

	sf := &kubearchivev1.SinkFilter{}
	err := client.Get(ctx, types.NamespacedName{Name: constants.SinkFilterResourceName, Namespace: constants.KubeArchiveNamespace}, sf)
	if errors.IsNotFound(err) {
		sf = desiredSinkFilterNamespace(ctx, nil, namespace, resources)
		err = client.Create(ctx, sf)
		if err != nil {
			slog.Error("Failed to create SinkFilter", "error", err, "name", constants.SinkFilterResourceName)
			return err
		}
		return nil
	} else if err != nil {
		slog.Error("Failed to reconcile SinkFilter", "error", err, "name", constants.SinkFilterResourceName)
		return err
	}

	sf = desiredSinkFilterNamespace(ctx, sf, namespace, resources)
	err = client.Update(ctx, sf)
	if err != nil {
		slog.Error("Failed to update SinkFilter", "error", err, "name", constants.SinkFilterResourceName)
		return err
	}
	return nil
}

func desiredSinkFilterNamespace(ctx context.Context, sf *kubearchivev1.SinkFilter, namespace string, resources []kubearchivev1.KubeArchiveConfigResource) *kubearchivev1.SinkFilter {

	slog.Info("in desiredSinkFilterNamespace")

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

	if resources != nil {
		sf.Spec.Namespaces[namespace] = resources
	} else {
		delete(sf.Spec.Namespaces, namespace)
	}

	// Note that the owner reference is NOT set on the SinkFilter resource.  It should not be deleted when
	// the KubeArchiveConfig object is deleted.
	return sf
}

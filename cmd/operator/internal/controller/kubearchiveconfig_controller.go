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
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;clusterroles;roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
//+kubebuilder:rbac:groups=sources.knative.dev,resources=apiserversources,verbs=create;delete;get;list;update;watch
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

			if err := r.reconcileVacuumBrokerRoleBinding(ctx, kaconfig, false); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.reconcileKubeArchiveClusterConfigReadClusterRoleBinding(ctx, kaconfig, false); err != nil {
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

	err = r.reconcileVacuumResources(ctx, kaconfig)
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

	role, err := r.reconcileRole(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveSinkName, createPolicyRules(ctx, r.Mapper, resources, []string{"delete"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, namespace string, name string, rules []rbacv1.PolicyRule) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRole " + name)
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

func (r *KubeArchiveConfigReconciler) reconcileA13eRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, role *rbacv1.ClusterRole) (*rbacv1.RoleBinding, error) {
	subject := newSubject(constants.KubeArchiveNamespace, role.Name)
	binding, err := r.reconcileRoleBinding(ctx, kaconfig, kaconfig.Namespace, role.Name, "ClusterRole", true, subject)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {
	subject := newSubject(constants.KubeArchiveNamespace, role.Name)
	binding, err := r.reconcileRoleBinding(ctx, kaconfig, kaconfig.Namespace, role.Name, "Role", true, subject)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, namespace string, name string, kind string, add bool, subjects ...rbacv1.Subject) (*rbacv1.RoleBinding, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRoleBinding " + name)
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
			log.Error(err, "Failed to create RoleBinding "+name)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile RoleBinding "+name)
		return nil, err
	}

	if add {
		existing.Subjects = mergeSubjects(existing.Subjects, desired.Subjects)
	} else {
		existing.Subjects = removeSubjects(existing.Subjects, desired.Subjects...)
	}
	err = r.Client.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update RoleBinding "+name)
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

func (r *KubeArchiveConfigReconciler) reconcileClusterRoleBinding(ctx context.Context, name string, kind string, add bool, subjects ...rbacv1.Subject) (*rbacv1.ClusterRoleBinding, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileClusterRoleBinding " + name)
	desired := r.desiredClusterRoleBinding(name, kind, subjects...)

	existing := &rbacv1.ClusterRoleBinding{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: name}, existing)
	if errors.IsNotFound(err) {
		err = r.Client.Create(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to create ClusterRoleBinding "+name)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile ClusterRoleBinding "+name)
		return nil, err
	}

	if add {
		existing.Subjects = mergeSubjects(existing.Subjects, desired.Subjects)
	} else {
		existing.Subjects = removeSubjects(existing.Subjects, desired.Subjects...)
	}
	err = r.Client.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update ClusterRoleBinding "+name)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) desiredClusterRoleBinding(name string, kind string, subjects ...rbacv1.Subject) *rbacv1.ClusterRoleBinding {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
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

func removeSubjects(subjects []rbacv1.Subject, removals ...rbacv1.Subject) []rbacv1.Subject {
	set := make(map[rbacv1.Subject]struct{})
	for _, s := range removals {
		set[s] = struct{}{}
	}

	result := []rbacv1.Subject{}
	for _, s := range subjects {
		if _, ok := set[s]; !ok {
			result = append(result, s)
		}
	}
	return result
}

func mergeSubjects(s1, s2 []rbacv1.Subject) []rbacv1.Subject {
	set := make(map[rbacv1.Subject]struct{})
	for _, s := range s1 {
		set[s] = struct{}{}
	}
	for _, s := range s2 {
		set[s] = struct{}{}
	}

	subjects := make([]rbacv1.Subject, len(set))
	i := 0
	for s := range set {
		subjects[i] = s
		i++
	}
	return subjects
}

func newSubject(namespace string, name string) rbacv1.Subject {
	return rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      name,
		Namespace: namespace,
	}
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

func (r *KubeArchiveConfigReconciler) reconcileVacuumResources(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) error {
	if _, err := r.reconcileClusterVacuumServiceAccount(ctx); err != nil {
		return err
	}

	if _, err := r.reconcileVacuumServiceAccount(ctx, kaconfig); err != nil {
		return err
	}

	role, err := r.reconcileVacuumRole(ctx, kaconfig)
	if err != nil {
		return err
	}

	if _, err := r.reconcileVacuumRoleBinding(ctx, kaconfig, role); err != nil {
		return err
	}

	if _, err := r.reconcileVacuumBrokerRole(ctx); err != nil {
		return err
	}

	if err := r.reconcileVacuumBrokerRoleBinding(ctx, kaconfig, true); err != nil {
		return err
	}

	if err := r.reconcileKubeArchiveClusterConfigReadClusterRoleBinding(ctx, kaconfig, true); err != nil {
		return err
	}
	return nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumServiceAccount(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumServiceAccount")

	sa, err := r.reconcileServiceAccount(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveVacuumName)
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileClusterVacuumServiceAccount(ctx context.Context) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileClusterVacuumServiceAccount")

	sa, err := r.reconcileServiceAccount(ctx, nil, constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName)
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileServiceAccount(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, namespace string, name string) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileServiceAccount")
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
			log.Error(err, "Failed to create ServiceAccount")
			return nil, err
		}
	} else if err != nil {
		log.Error(err, "Failed to reconcile ServiceAccount")
		return nil, err
	}

	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumRole")

	resources := make([]sourcesv1.APIVersionKindSelector, 0)
	for _, kar := range kaconfig.Spec.Resources {
		resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
		resources = append(resources, resource)
	}

	role, err := r.reconcileRole(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveVacuumName, createPolicyRules(ctx, r.Mapper, resources, []string{"get", "list"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumRoleBinding")

	ns := newSubject(kaconfig.Namespace, constants.KubeArchiveVacuumName)
	ka := newSubject(constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName)
	binding, err := r.reconcileRoleBinding(ctx, kaconfig, kaconfig.Namespace, role.Name, "Role", true, ns, ka)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileKubeArchiveClusterConfigReadClusterRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, add bool) error {
	log := log.FromContext(ctx)

	log.Info("in reconcileKubeArchiveClusterConfigReadClusterRoleBinding")

	subjects := []rbacv1.Subject{}
	subjects = append(subjects, newSubject(kaconfig.Namespace, constants.KubeArchiveVacuumName))
	if add {
		subjects = append(subjects, newSubject(constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName))
	}
	_, err := r.reconcileClusterRoleBinding(ctx, constants.ClusterKubeArchiveConfigClusterRoleBindingName, "ClusterRole", add, subjects...)
	if err != nil {
		return err
	}
	return nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumBrokerRole(ctx context.Context) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumBrokerRole")

	resources := []sourcesv1.APIVersionKindSelector{
		sourcesv1.APIVersionKindSelector{
			APIVersion: "eventing.knative.dev/v1",
			Kind:       "Broker",
		},
	}
	role, err := r.reconcileRole(ctx, nil, constants.KubeArchiveNamespace, constants.KubeArchiveVacuumBroker, createPolicyRules(ctx, r.Mapper, resources, []string{"get", "list"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumBrokerRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, add bool) error {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumBrokerRoleBinding")

	subjects := []rbacv1.Subject{}
	subjects = append(subjects, newSubject(kaconfig.Namespace, constants.KubeArchiveVacuumName))
	if add {
		subjects = append(subjects, newSubject(constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName))
	}
	_, err := r.reconcileRoleBinding(ctx, nil, constants.KubeArchiveNamespace, constants.KubeArchiveVacuumBroker, "Role", add, subjects...)
	if err != nil {
		return err
	}
	return nil
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	log := log.FromContext(ctx)

	log.Info("Reconciling KubeArchiveConfig")

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

			log.Info("Deleting KubeArchiveConfig")

			if err := reconcileSinkFilter(ctx, r.Client, kaconfig.Namespace, nil); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.reconcileKubeArchiveClusterConfigReadClusterRoleBinding(ctx, kaconfig, false); err != nil {
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

	if err := reconcileSinkFilter(ctx, r.Client, kaconfig.Namespace, kaconfig.Spec.Resources); err != nil {
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
	log := log.FromContext(ctx)

	resources := make([]sourcesv1.APIVersionKindSelector, 0)
	for _, kar := range kaconfig.Spec.Resources {
		resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
		resources = append(resources, resource)
	}

	ckac := &kubearchivev1.ClusterKubeArchiveConfig{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.KubeArchiveConfigResourceName}, ckac)
	if err == nil {
		for _, kar := range ckac.Spec.Resources {
			resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
			resources = append(resources, resource)
		}
	} else if !errors.IsNotFound(err) {
		log.Error(err, "Unable to get ClusterKubeArchiveConfg when reconciling sink role ")
	}

	role, err := r.reconcileRole(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveSinkName, createPolicyRules(ctx, r.Mapper, resources, []string{"delete"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, namespace string, name string, rules []rbacv1.PolicyRule) (*rbacv1.Role, error) {
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

func (r *KubeArchiveConfigReconciler) reconcileSinkRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {
	subject := newSubject(constants.KubeArchiveNamespace, role.Name)
	binding, err := r.reconcileRoleBinding(ctx, kaconfig, kaconfig.Namespace, role.Name, "Role", true, subject)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, namespace string, name string, kind string, add bool, subjects ...rbacv1.Subject) (*rbacv1.RoleBinding, error) {
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

func (r *KubeArchiveConfigReconciler) reconcileVacuumResources(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) error {
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

	if err := r.reconcileKubeArchiveClusterConfigReadClusterRoleBinding(ctx, kaconfig, true); err != nil {
		return err
	}
	return nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumServiceAccount(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumServiceAccount")

	sa, err := r.reconcileServiceAccount(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveVacuumName)
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileServiceAccount(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, namespace string, name string) (*corev1.ServiceAccount, error) {
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

func (r *KubeArchiveConfigReconciler) reconcileVacuumRole(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileVacuumRole")

	resources := []sourcesv1.APIVersionKindSelector{
		{
			APIVersion: "kubearchive.org/v1",
			Kind:       "KubeArchiveConfig",
		},
		{
			APIVersion: "kubearchive.org/v1",
			Kind:       "NamespaceVacuumConfig",
		},
	}

	for _, kar := range kaconfig.Spec.Resources {
		resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
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
			resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
			resources = append(resources, resource)
		}
	}

	role, err := r.reconcileRole(ctx, kaconfig, kaconfig.Namespace, constants.KubeArchiveVacuumName, createPolicyRules(ctx, r.Mapper, resources, []string{"get", "list"}))
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *KubeArchiveConfigReconciler) reconcileVacuumRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, role *rbacv1.Role) (*rbacv1.RoleBinding, error) {
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

func (r *KubeArchiveConfigReconciler) reconcileKubeArchiveClusterConfigReadClusterRoleBinding(ctx context.Context, kaconfig *kubearchivev1.KubeArchiveConfig, add bool) error {
	log := log.FromContext(ctx)

	log.Info("in reconcileKubeArchiveClusterConfigReadClusterRoleBinding")

	subjects := []rbacv1.Subject{newSubject(kaconfig.Namespace, constants.KubeArchiveVacuumName)}
	if add {
		subjects = append(subjects, newSubject(constants.KubeArchiveNamespace, constants.KubeArchiveClusterVacuumName))
	}
	_, err := r.reconcileClusterRoleBinding(ctx, constants.ClusterKubeArchiveConfigClusterRoleBindingName, "ClusterRole", add, subjects...)
	if err != nil {
		return err
	}
	return nil
}

func createPolicyRules(ctx context.Context, mapper meta.RESTMapper, resources []sourcesv1.APIVersionKindSelector, verbs []string) []rbacv1.PolicyRule {
	log := log.FromContext(ctx)
	groups := make(map[string][]string)

	for _, resource := range resources {
		apiGroup := ""
		apiVersion := resource.APIVersion
		data := strings.Split(apiVersion, "/")
		if len(data) > 1 {
			apiGroup = data[0]
			apiVersion = data[1]
		}
		// The resource field in the GVR contains the plural version of the resource, and
		// the kubernetes Role expects this lower-cased plural version.
		gvr, err := mapper.RESTMapping(schema.GroupKind{Group: apiGroup, Kind: resource.Kind}, apiVersion)
		if err == nil {
			if _, exists := groups[apiGroup]; !exists {
				groups[apiGroup] = make([]string, 0)
			}
			groups[apiGroup] = append(groups[apiGroup], strings.ToLower(gvr.Resource.Resource))
		} else {
			log.Error(err, "Failed to get GVR for "+resource.APIVersion)
		}
	}

	var rules []rbacv1.PolicyRule
	for group, resList := range groups {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: []string{group},
			Resources: resList,
			Verbs:     verbs})
	}

	return rules
}

func desiredServiceAccount(namespace string, name string) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return sa
}

const (
	resourceFinalizerName = "kubearchive.org/finalizer"
)

// reconcileSinkFilter reconciles the SinkFilter resource.
// Note: This function is also used by clusterkubearchiveconfig_controller.go
func reconcileSinkFilter(ctx context.Context, client client.Client, namespace string, resources []kubearchivev1.KubeArchiveConfigResource) error {
	log := log.FromContext(ctx)

	log.Info("in reconcileSinkFilter")

	sf := &kubearchivev1.SinkFilter{}
	err := client.Get(ctx, types.NamespacedName{Name: constants.SinkFilterResourceName, Namespace: constants.KubeArchiveNamespace}, sf)
	if errors.IsNotFound(err) {
		sf = desiredSinkFilter(ctx, nil, namespace, resources)
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

	sf = desiredSinkFilter(ctx, sf, namespace, resources)
	err = client.Update(ctx, sf)
	if err != nil {
		log.Error(err, "Failed to update SinkFilter "+constants.SinkFilterResourceName)
		return err
	}
	return nil
}

func desiredSinkFilter(ctx context.Context, sf *kubearchivev1.SinkFilter, namespace string, resources []kubearchivev1.KubeArchiveConfigResource) *kubearchivev1.SinkFilter {
	log := log.FromContext(ctx)

	log.Info("in desiredSinkFilter")

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

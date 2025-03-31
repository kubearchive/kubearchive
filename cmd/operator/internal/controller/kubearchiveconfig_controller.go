// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a13e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
)

const (
	SinkFilterConfigMapName   = "sink-filters"
	ApiServerSourceLabelName  = "kubearchive.org/enabled"
	ApiServerSourceLabelValue = "true"
)

var (
	k9eNs         = os.Getenv("KUBEARCHIVE_NAMESPACE")
	a13eName      = k9eNs + "-a13e"
	k9eSinkName   = "kubearchive-sink"
	k9eBrokerName = "kubearchive-broker"
)

// KubeArchiveConfigReconciler reconciles a KubeArchiveConfig object
type KubeArchiveConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Mapper meta.RESTMapper
}

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
	if err := r.Get(ctx, req.NamespacedName, kaconfig); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue (we need
		// to wait for a new notification), and we can get them on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := "kubearchive.org/finalizer"

	if kaconfig.DeletionTimestamp.IsZero() {
		// The object is not being deleted, add the finalizer if necessary.
		if !controllerutil.ContainsFinalizer(kaconfig, finalizerName) {
			controllerutil.AddFinalizer(kaconfig, finalizerName)
			if err := r.Update(ctx, kaconfig); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted.
		if controllerutil.ContainsFinalizer(kaconfig, finalizerName) {
			// Finalizer is present, clean up filters from ConfigMap and remove Namespace label.

			log.Info("Deleting KubeArchiveConfig")

			// Reconcile all a13e resources to clean filters and potentially delete the a13e instance.
			err := r.cleanupK9eResources(ctx, kaconfig)
			if err != nil {
				return ctrl.Result{}, err
			}

			if err := r.removeNamespaceLabel(ctx, kaconfig); err != nil {
				// If label removal fails, return with error so that it can be retried.
				return ctrl.Result{}, err
			}

			// Remove the finalizer from the list and update it.
			controllerutil.RemoveFinalizer(kaconfig, finalizerName)
			if err := r.Update(ctx, kaconfig); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the resource is being deleted.
		return ctrl.Result{}, nil
	}

	cm, err := r.reconcileFilterConfigMap(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	resources := r.parseConfigMap(ctx, cm)

	_, err = r.reconcileA13eServiceAccount(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterrole, err := r.reconcileA13eRole(ctx, resources)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileA13eRoleBinding(ctx, kaconfig, clusterrole)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(resources) > 0 {
		err = r.reconcileA13e(ctx, resources)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.Info("No resources, not reconciling ApiServerSource")
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

func (r *KubeArchiveConfigReconciler) cleanupK9eResources(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) error {
	log := log.FromContext(ctx)

	log.Info("in cleanupK9eResources")

	cm, err := r.deleteNamespaceFromFilterConfigMap(ctx, kaconfig)
	if err != nil {
		return err
	}
	resources := r.parseConfigMap(ctx, cm)

	r.deleteRoleBinding(ctx, kaconfig, k9eSinkName, "Role")
	r.deleteRoleBinding(ctx, kaconfig, a13eName, "ClusterRole")
	r.deleteSinkRole(ctx, kaconfig)

	if len(resources) > 0 {
		_, err := r.reconcileA13eRole(ctx, resources)
		if err != nil {
			return err
		}

		err = r.reconcileA13e(ctx, resources)
		if err != nil {
			return err
		}
	} else {
		log.Info("No resources specified, deleting ApiServerSource.")
		// Leave ApiServerSource ClusterRole and ServiceAccount around as RoleBindings could be
		// referring to both.
		r.deleteA13e(ctx)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeArchiveConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1alpha1.KubeArchiveConfig{}).
		//Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func (r *KubeArchiveConfigReconciler) reconcileA13eServiceAccount(ctx context.Context) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileServiceAccount")
	sa := &corev1.ServiceAccount{}

	err := r.Get(ctx, types.NamespacedName{Name: a13eName, Namespace: k9eNs}, sa)
	if errors.IsNotFound(err) {
		err = r.Create(ctx, r.desiredA13eServiceAccount())
		if err != nil {
			log.Error(err, "Failed to create ServiceAccount")
			return sa, err
		}
	} else if err != nil {
		log.Error(err, "Failed to reconcile ServiceAccount")
		return sa, err
	}

	return sa, nil
}

func (r *KubeArchiveConfigReconciler) desiredA13eServiceAccount() *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a13eName,
			Namespace: k9eNs,
		},
	}
	return sa
}

func (r *KubeArchiveConfigReconciler) reconcileA13eRole(ctx context.Context, resources []sourcesv1.APIVersionKindSelector) (*rbacv1.ClusterRole, error) {
	return r.reconcileClusterRole(ctx, a13eName, r.getRules(ctx, resources, []string{"get", "list", "watch"}))
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	resources := make([]sourcesv1.APIVersionKindSelector, 0)
	for _, kar := range kaconfig.Spec.Resources {
		resource := sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
		resources = append(resources, resource)
	}
	return r.reconcileRole(ctx, kaconfig, k9eSinkName, r.getRules(ctx, resources, []string{"delete"}))
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string, rules []rbacv1.PolicyRule) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRole " + roleName)
	desired, err := r.desiredRole(kaconfig, roleName, rules)
	if err != nil {
		log.Error(err, "Unable to get desired Role "+roleName)
		return nil, err
	}

	existing := &rbacv1.Role{}
	err = r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: kaconfig.Namespace}, existing)
	if errors.IsNotFound(err) {
		err = r.Create(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to create Role "+roleName)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile Role "+roleName)
		return nil, err
	}

	existing.Rules = desired.Rules
	err = r.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update Role "+roleName)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) deleteSinkRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) {
	r.deleteRole(ctx, kaconfig, k9eSinkName)
}

func (r *KubeArchiveConfigReconciler) deleteRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string) {
	log := log.FromContext(ctx)

	log.Info("in deleteRole " + roleName)
	role, err := r.desiredRole(kaconfig, roleName, []rbacv1.PolicyRule{})
	if err != nil {
		log.Error(err, "Unable to get desired Role "+roleName)
		return
	}

	err = r.Delete(ctx, role)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Role "+roleName)
	}
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

func (r *KubeArchiveConfigReconciler) reconcileClusterRole(ctx context.Context, roleName string, rules []rbacv1.PolicyRule) (*rbacv1.ClusterRole, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileClusterRole " + roleName)

	desired := r.desiredClusterRole(roleName, rules)
	existing := &rbacv1.ClusterRole{}
	err := r.Get(ctx, types.NamespacedName{Name: roleName}, existing)
	if errors.IsNotFound(err) {
		err = r.Create(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to create ClusterRole "+roleName)
			return nil, err
		}
		return desired, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile ClusterRole "+roleName)
		return nil, err
	}

	existing.Rules = desired.Rules
	err = r.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update ClusterRole "+roleName)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) desiredClusterRole(roleName string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
		},
		Rules: rules,
	}

	return role
}

func (r *KubeArchiveConfigReconciler) getRules(ctx context.Context, resources []sourcesv1.APIVersionKindSelector, verbs []string) []rbacv1.PolicyRule {
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
		gvr, err := r.Mapper.RESTMapping(schema.GroupKind{Group: apiGroup, Kind: resource.Kind}, apiVersion)
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
	err = r.Get(ctx, types.NamespacedName{Name: name, Namespace: kaconfig.Namespace}, existing)
	if errors.IsNotFound(err) {
		err = r.Create(ctx, desired)
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
	err = r.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update RoleBinding "+name)
		return nil, err
	}
	return existing, nil
}

func (r *KubeArchiveConfigReconciler) deleteRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, name string, kind string) {
	log := log.FromContext(ctx)

	log.Info("in deleteRoleBinding " + name)
	binding, err := r.desiredRoleBinding(kaconfig, name, kind)
	if err != nil {
		log.Error(err, "Unable to get desired RoleBinding "+name)
		return
	}
	err = r.Delete(ctx, binding)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete RoleBinding "+name)
	}
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
			Namespace: k9eNs,
		}},
	}

	if err := ctrl.SetControllerReference(kaconfig, binding, r.Scheme); err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileA13e(ctx context.Context, resources []sourcesv1.APIVersionKindSelector) error {
	log := log.FromContext(ctx)

	log.Info("in reconcileApiServerSource")
	desired := r.desiredA13e(resources)

	existing := &sourcesv1.ApiServerSource{}
	err := r.Get(ctx, types.NamespacedName{Name: a13eName, Namespace: k9eNs}, existing)
	if errors.IsNotFound(err) {
		err = r.Create(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to create ApiServerSource")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile ApiServerSource")
		return err
	}

	existing.Spec = desired.Spec
	err = r.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update ApiServerSource")
		return err
	}
	return nil
}

func (r *KubeArchiveConfigReconciler) deleteA13e(ctx context.Context) {
	log := log.FromContext(ctx)

	log.Info("in deleteApiServerSource")

	existing := &sourcesv1.ApiServerSource{}
	err := r.Get(ctx, types.NamespacedName{Name: a13eName, Namespace: k9eNs}, existing)
	if errors.IsNotFound(err) {
		log.Info("No ApiServerSource to delete")
	} else if err != nil {
		log.Error(err, "Error retrieving ApiServerSource")
	}

	err = r.Delete(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to delete ApiServerSource")
	}
}

func (r *KubeArchiveConfigReconciler) parseConfigMap(ctx context.Context, cm *corev1.ConfigMap) []sourcesv1.APIVersionKindSelector {
	log := log.FromContext(ctx)

	resourceMap := map[string]sourcesv1.APIVersionKindSelector{}
	resourceKeys := make([]string, 0)
	for namespace, yaml := range cm.Data {
		kars, err := kubearchivev1alpha1.LoadFromString(yaml)
		if err != nil {
			log.Error(err, "Failed to load KubeArchiveConfigResource for namespace "+namespace)
			continue
		}
		for _, kar := range kars {
			key := kar.Selector.Kind + "-" + kar.Selector.APIVersion
			if _, ok := resourceMap[key]; !ok {
				// One include kind and apiVersion, ignore all other filter information.
				resourceMap[key] = sourcesv1.APIVersionKindSelector{Kind: kar.Selector.Kind, APIVersion: kar.Selector.APIVersion}
				resourceKeys = append(resourceKeys, key)
			}
		}
	}
	sort.Strings(resourceKeys)

	resources := make([]sourcesv1.APIVersionKindSelector, 0, len(resourceMap))
	for _, key := range resourceKeys {
		resources = append(resources, resourceMap[key])
	}
	return resources
}

func (r *KubeArchiveConfigReconciler) desiredA13e(resources []sourcesv1.APIVersionKindSelector) *sourcesv1.ApiServerSource {

	source := &sourcesv1.ApiServerSource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiServerSource",
			APIVersion: "sources.knative.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      a13eName,
			Namespace: k9eNs,
		},
		Spec: sourcesv1.ApiServerSourceSpec{
			EventMode:          "Resource",
			ServiceAccountName: a13eName,
			Resources:          resources,
			SourceSpec: duckv1.SourceSpec{
				Sink: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: "eventing.knative.dev/v1",
						Kind:       "Broker",
						Name:       k9eBrokerName,
						Namespace:  k9eNs,
					},
				},
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{ApiServerSourceLabelName: ApiServerSourceLabelValue},
			},
		},
	}

	return source
}

func (r *KubeArchiveConfigReconciler) reconcileFilterConfigMap(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileFilterConfigMap")

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: SinkFilterConfigMapName, Namespace: k9eNs}, cm)
	if errors.IsNotFound(err) {
		cm, err = r.desiredFilterConfigMap(ctx, kaconfig, nil)
		if err != nil {
			log.Error(err, "Unable to get desired filter ConfigMap "+SinkFilterConfigMapName)
			return nil, err
		}
		err = r.Create(ctx, cm)
		if err != nil {
			log.Error(err, "Failed to create filter ConfigMap "+SinkFilterConfigMapName)
			return nil, err
		}
		return cm, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile filter ConfigMap "+SinkFilterConfigMapName)
		return nil, err
	}

	cm, err = r.desiredFilterConfigMap(ctx, kaconfig, cm)
	if err != nil {
		log.Error(err, "Unable to get desired ConfigMap "+SinkFilterConfigMapName)
		return nil, err
	}
	err = r.Update(ctx, cm)
	if err != nil {
		log.Error(err, "Failed to update filter ConfigMap "+SinkFilterConfigMapName)
		return nil, err
	}
	return cm, nil
}

func (r *KubeArchiveConfigReconciler) desiredFilterConfigMap(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	log.Info("in desiredFilterConfigMap")

	if cm == nil {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SinkFilterConfigMapName,
				Namespace: k9eNs,
			},
			Data: map[string]string{},
		}
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	yamlBytes, err := yaml.Marshal(kaconfig.Spec.Resources)
	if err != nil {
		log.Error(err, "Failed to convert KubeArchiveConfig resources to JSON")
		return cm, err
	}

	cm.Data[kaconfig.Namespace] = string(yamlBytes)

	// Note that the owner reference is NOT set on the ConfigMap.  It should not be deleted when
	// the KubeArchiveConfig object is deleted.
	return cm, nil
}

func (r *KubeArchiveConfigReconciler) deleteNamespaceFromFilterConfigMap(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	log.Info("in deleteNamespaceFromFilterConfigMap")

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: SinkFilterConfigMapName, Namespace: k9eNs}, cm)
	if err != nil {
		log.Error(err, "Failed to get filter ConfigMap "+SinkFilterConfigMapName)
		return nil, err
	}

	delete(cm.Data, kaconfig.Namespace)
	err = r.Update(ctx, cm)
	if err != nil {
		log.Error(err, "Failed to remove namespace '"+kaconfig.Namespace+"' from filter ConfigMap "+SinkFilterConfigMapName)
		return nil, err
	}
	return cm, nil
}

func (r *KubeArchiveConfigReconciler) reconcileNamespace(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.Namespace, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileNamespace")

	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: kaconfig.Namespace}, ns)
	if err != nil {
		log.Error(err, "Failed to get Namespace "+kaconfig.Namespace)
		return nil, err
	}

	ns.Labels[ApiServerSourceLabelName] = ApiServerSourceLabelValue
	err = r.Update(ctx, ns)
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
	err := r.Get(ctx, types.NamespacedName{Name: kaconfig.Namespace}, ns)
	if err != nil {
		log.Error(err, "Failed to get Namespace "+kaconfig.Namespace)
		return err
	}

	delete(ns.Labels, ApiServerSourceLabelName)

	err = r.Update(ctx, ns)
	if err != nil {
		log.Error(err, "Failed to update Namespace "+kaconfig.Namespace)
		return err
	}

	return nil
}

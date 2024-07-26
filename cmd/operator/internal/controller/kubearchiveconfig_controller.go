// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a14e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
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

const SinkFilterConfigMapName = "sink-filters"

var k9eNs = os.Getenv("KUBEARCHIVE_NAMESPACE")
var a14eName = k9eNs + "-a14e"
var k9eSinkName = k9eNs + "-sink"

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
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=bind;create;delete;escalate;get;list;update;watch
//+kubebuilder:rbac:groups=sources.knative.dev,resources=apiserversources,verbs=create;delete;get;list;update;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=create;delete;get;list;update;watch

func (r *KubeArchiveConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("KubeArchiveConfig reconciling.")

	kaconfig := &kubearchivev1alpha1.KubeArchiveConfig{}
	if err := r.Get(ctx, req.NamespacedName, kaconfig); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue (we need
		// to wait for a new notification), and we can get them on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := "kubearchive.org/finalizer"

	if kaconfig.ObjectMeta.DeletionTimestamp.IsZero() {
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
			// Finalizer is present, clean up filters from ConfigMap.
			if err := r.removeFilters(ctx, kaconfig); err != nil {
				// If filter deletion fails, return with error so that it can be retried.
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

	_, err := r.reconcileA14eServiceAccount(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileA14eRole(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileA14eRoleBinding(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileA14e(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileSinkRole(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileSinkRoleBinding(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.reconcileFilterConfigMap(ctx, kaconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

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

func (r *KubeArchiveConfigReconciler) reconcileA14eServiceAccount(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileServiceAccount")
	sa, err := r.desiredA14eServiceAccount(kaconfig)
	if err != nil {
		log.Error(err, "Unable to get desired ServiceAccount")
		return sa, err
	}

	err = r.Get(ctx, types.NamespacedName{Name: a14eName, Namespace: kaconfig.Namespace}, &corev1.ServiceAccount{})
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

func (r *KubeArchiveConfigReconciler) desiredA14eServiceAccount(kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a14eName,
			Namespace: kaconfig.Namespace,
		},
	}

	if err := ctrl.SetControllerReference(kaconfig, sa, r.Scheme); err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *KubeArchiveConfigReconciler) reconcileA14eRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	return r.reconcileRole(ctx, kaconfig, a14eName, []string{"get", "list", "watch"})
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.Role, error) {
	return r.reconcileRole(ctx, kaconfig, k9eSinkName, []string{"delete"})
}

func (r *KubeArchiveConfigReconciler) reconcileRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string, verbs []string) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRole " + roleName)
	role, err := r.desiredRole(ctx, kaconfig, roleName, verbs)
	if err != nil {
		log.Error(err, "Unable to get desired Role "+roleName)
		return role, err
	}

	err = r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: kaconfig.Namespace}, &rbacv1.Role{})
	if err == nil {
		err = r.Update(ctx, role)
		if err != nil {
			log.Error(err, "Failed to update Role "+roleName)
			return role, err
		}
	} else if errors.IsNotFound(err) {
		err = r.Create(ctx, role)
		if err != nil {
			log.Error(err, "Failed to create Role "+roleName)
			return role, err
		}
	} else {
		log.Error(err, "Failed to reconcile Role "+roleName)
		return role, err
	}

	return role, nil
}

func (r *KubeArchiveConfigReconciler) desiredRole(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string, verbs []string) (*rbacv1.Role, error) {
	log := log.FromContext(ctx)
	var rules []rbacv1.PolicyRule
	for _, resource := range kaconfig.Spec.Resources {
		apiGroup := ""
		apiVersion := resource.Selector.APIVersion
		data := strings.Split(resource.Selector.APIVersion, "/")
		if len(data) > 1 {
			apiGroup = data[0]
			apiVersion = data[1]
		}
		// The resource field in the GVR contains the plural version of the resource, and
		// the kubernetes Role expects this lower-cased plural version.
		gvr, err := r.Mapper.RESTMapping(schema.GroupKind{Group: apiGroup, Kind: resource.Selector.Kind}, apiVersion)
		if err == nil {
			rules = append(rules, rbacv1.PolicyRule{
				APIGroups: []string{apiGroup},
				Resources: []string{strings.ToLower(gvr.Resource.Resource)},
				Verbs:     verbs})
		} else {
			log.Error(err, "Failed to get GVR for "+resource.Selector.APIVersion)
		}
	}
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

func (r *KubeArchiveConfigReconciler) reconcileA14eRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.RoleBinding, error) {
	return r.reconcileRoleBinding(ctx, kaconfig, a14eName, a14eName, kaconfig.Namespace)
}

func (r *KubeArchiveConfigReconciler) reconcileSinkRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*rbacv1.RoleBinding, error) {
	return r.reconcileRoleBinding(ctx, kaconfig, k9eSinkName, k9eSinkName, k9eNs)
}

func (r *KubeArchiveConfigReconciler) reconcileRoleBinding(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string, subjectName string, subjectNamespace string) (*rbacv1.RoleBinding, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileRoleBinding " + roleName)
	binding, err := r.desiredRoleBinding(kaconfig, roleName, subjectName, subjectNamespace)
	if err != nil {
		log.Error(err, "Unable to get desired RoleBinding "+roleName)
		return binding, err
	}

	err = r.Get(ctx, types.NamespacedName{Name: roleName, Namespace: kaconfig.Namespace}, &rbacv1.RoleBinding{})
	if err == nil {
		err = r.Update(ctx, binding)
		if err != nil {
			log.Error(err, "Failed to update RoleBinding "+roleName)
			return binding, err
		}
	} else if errors.IsNotFound(err) {
		err = r.Create(ctx, binding)
		if err != nil {
			log.Error(err, "Failed to create RoleBinding "+roleName)
			return binding, err
		}
	} else {
		log.Error(err, "Failed to reconcile RoleBinding "+roleName)
		return binding, err
	}

	return binding, nil
}

func (r *KubeArchiveConfigReconciler) desiredRoleBinding(kaconfig *kubearchivev1alpha1.KubeArchiveConfig, roleName string, subjectName string, subjectNamespace string) (*rbacv1.RoleBinding, error) {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: kaconfig.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      subjectName,
			Namespace: subjectNamespace,
		}},
	}

	if err := ctrl.SetControllerReference(kaconfig, binding, r.Scheme); err != nil {
		return nil, err
	}
	return binding, nil
}

func (r *KubeArchiveConfigReconciler) reconcileA14e(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*sourcesv1.ApiServerSource, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileApiServerSource")
	source, err := r.desiredA14e(kaconfig)
	if err != nil {
		log.Error(err, "Unable to get desired ApiServerSource")
		return source, err
	}

	existing := &sourcesv1.ApiServerSource{}
	err = r.Get(ctx, types.NamespacedName{Name: a14eName, Namespace: kaconfig.Namespace}, existing)
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

func (r *KubeArchiveConfigReconciler) desiredA14e(kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*sourcesv1.ApiServerSource, error) {

	resources := make([]sourcesv1.APIVersionKindSelector, 0)
	for _, resource := range kaconfig.Spec.Resources {
		resources = append(resources, resource.Selector)
	}
	source := &sourcesv1.ApiServerSource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiServerSource",
			APIVersion: "sources.knative.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      a14eName,
			Namespace: kaconfig.Namespace,
		},
		Spec: sourcesv1.ApiServerSourceSpec{
			EventMode:          "Resource",
			ServiceAccountName: a14eName,
			Resources:          resources,
			SourceSpec: duckv1.SourceSpec{
				Sink: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: "v1",
						Kind:       "Service",
						Name:       k9eSinkName,
						Namespace:  k9eNs,
					},
				},
			},
			Filters: []eventingv1.SubscriptionsAPIFilter{
				{
					CESQL: "type NOT LIKE '%delete'", // a14e SHOULD NOT send delete cloudevents
				},
			},
		},
	}

	return source, nil
}

func (r *KubeArchiveConfigReconciler) reconcileFilterConfigMap(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) (*corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileFilterConfigMap")

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: SinkFilterConfigMapName, Namespace: k9eNs}, cm)
	if err == nil {
		cm, err := r.desiredFilterConfigMap(ctx, kaconfig, cm)
		if err != nil {
			log.Error(err, "Unable to get desired ConfigMap "+SinkFilterConfigMapName)
			return cm, err
		}
		err = r.Update(ctx, cm)
		if err != nil {
			log.Error(err, "Failed to update filter ConfigMap "+SinkFilterConfigMapName)
			return cm, err
		}
	} else if errors.IsNotFound(err) {
		cm, err := r.desiredFilterConfigMap(ctx, kaconfig, nil)
		if err != nil {
			log.Error(err, "Unable to get desired filter ConfigMap "+SinkFilterConfigMapName)
			return cm, err
		}
		err = r.Create(ctx, cm)
		if err != nil {
			log.Error(err, "Failed to create filter ConfigMap "+SinkFilterConfigMapName)
			return cm, err
		}
	} else {
		log.Error(err, "Failed to reconcile filter ConfigMap "+SinkFilterConfigMapName)
		return cm, err
	}

	return cm, nil
}

func (r *KubeArchiveConfigReconciler) desiredFilterConfigMap(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig, cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	if cm == nil {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SinkFilterConfigMapName,
				Namespace: k9eNs,
			},
			Data: map[string]string{},
		}
	}

	yamlBytes, err := yaml.Marshal(kaconfig.Spec.Resources)
	if err != nil {
		log.Error(err, "Failed to convert KubeArchiveConfig resources to JSON")
		return cm, err
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	cm.Data[kaconfig.Namespace] = string(yamlBytes)

	// Note that the owner reference is NOT set on the ConfigMap.  It should not be deleted when
	// the KubeArchiveConfig object is deleted.
	return cm, nil
}

func (r *KubeArchiveConfigReconciler) removeFilters(ctx context.Context, kaconfig *kubearchivev1alpha1.KubeArchiveConfig) error {
	log := log.FromContext(ctx)

	log.Info("in removeFilters")

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: SinkFilterConfigMapName, Namespace: k9eNs}, cm)
	if err != nil {
		log.Error(err, "Unable to get desired ConfigMap "+SinkFilterConfigMapName)
		return err
	}

	delete(cm.Data, kaconfig.Namespace)

	err = r.Update(ctx, cm)
	if err != nil {
		log.Error(err, "Failed to update filter ConfigMap "+SinkFilterConfigMapName)
		return err
	}

	return nil
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a13e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"
	"encoding/json"
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
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

const (
	ApiServerSourceLabelName  = "kubearchive.org/enabled"
	ApiServerSourceLabelValue = "true"
	resourceFinalizerName     = "kubearchive.org/finalizer"
)

var (
	a13eName = constants.KubeArchiveNamespace + "-a13e"
)

func reconcileAllCommonResources(ctx context.Context, client client.Client, mapper meta.RESTMapper, namespace string, resources []kubearchivev1alpha1.KubeArchiveConfigResource) (*rbacv1.ClusterRole, error) {
	log := log.FromContext(ctx)

	log.Info("in ReconcileAllA13eCommonResources")

	var err error
	var sf *kubearchivev1alpha1.SinkFilter
	if sf, err = reconcileSinkFilter(ctx, client, namespace, resources); err != nil {
		return nil, err
	}
	sfres := getSinkFilterResources(sf)

	if _, err = reconcileServiceAccount(ctx, client, constants.KubeArchiveNamespace, a13eName); err != nil {
		return nil, err
	}

	var clusterrole *rbacv1.ClusterRole
	if clusterrole, err = reconcileA13eRole(ctx, client, mapper, sfres); err != nil {
		return nil, err
	}

	if err = reconcileA13e(ctx, client, sfres); err != nil {
		return nil, err
	}

	return clusterrole, nil
}

func reconcileServiceAccount(ctx context.Context, client client.Client, namespace string, name string) (*corev1.ServiceAccount, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileServiceAccount")
	sa := &corev1.ServiceAccount{}

	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sa)
	if errors.IsNotFound(err) {
		err = client.Create(ctx, desiredServiceAccount(namespace, name))
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

func desiredServiceAccount(namespace string, name string) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return sa
}

func reconcileA13eRole(ctx context.Context, client client.Client, mapper meta.RESTMapper, resources []sourcesv1.APIVersionKindSelector) (*rbacv1.ClusterRole, error) {
	return reconcileClusterRole(ctx, client, a13eName, createPolicyRules(ctx, mapper, resources, []string{"get", "list", "watch"}))
}

func reconcileClusterRole(ctx context.Context, client client.Client, roleName string, rules []rbacv1.PolicyRule) (*rbacv1.ClusterRole, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileClusterRole " + roleName)

	desired := desiredClusterRole(roleName, rules)
	existing := &rbacv1.ClusterRole{}
	err := client.Get(ctx, types.NamespacedName{Name: roleName}, existing)
	if errors.IsNotFound(err) {
		err = client.Create(ctx, desired)
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
	err = client.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update ClusterRole "+roleName)
		return nil, err
	}
	return existing, nil
}

func desiredClusterRole(name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
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

func reconcileA13e(ctx context.Context, client client.Client, resources []sourcesv1.APIVersionKindSelector) error {
	log := log.FromContext(ctx)

	log.Info("in reconcileApiServerSource")
	desired := desiredA13e(resources)

	existing := &sourcesv1.ApiServerSource{}
	err := client.Get(ctx, types.NamespacedName{Name: a13eName, Namespace: constants.KubeArchiveNamespace}, existing)
	if errors.IsNotFound(err) {
		err = client.Create(ctx, desired)
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
	err = client.Update(ctx, existing)
	if err != nil {
		log.Error(err, "Failed to update ApiServerSource")
		return err
	}
	return nil
}

func desiredA13e(resources []sourcesv1.APIVersionKindSelector) *sourcesv1.ApiServerSource {
	if len(resources) == 0 {
		// Make sure there's at least one entry to the ApiServerSource starts.
		resources = append(resources, sourcesv1.APIVersionKindSelector{Kind: "ClusterKubeArchiveConfig", APIVersion: "kubearchive.kubearchive.org/v1alpha1"})
	}

	source := &sourcesv1.ApiServerSource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiServerSource",
			APIVersion: "sources.knative.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      a13eName,
			Namespace: constants.KubeArchiveNamespace,
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
						Name:       constants.KubeArchiveBrokerName,
						Namespace:  constants.KubeArchiveNamespace,
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

func getSinkFilterResources(sf *kubearchivev1alpha1.SinkFilter) []sourcesv1.APIVersionKindSelector {
	resourceMap := map[string]sourcesv1.APIVersionKindSelector{}
	resourceKeys := make([]string, 0)
	for _, resources := range sf.Spec.Namespaces {
		for _, resource := range resources {
			key := resource.Selector.Kind + "-" + resource.Selector.APIVersion
			if _, ok := resourceMap[key]; !ok {
				// Only include kind and apiVersion, ignore all other filter information.
				resourceMap[key] = sourcesv1.APIVersionKindSelector{Kind: resource.Selector.Kind, APIVersion: resource.Selector.APIVersion}
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

func reconcileSinkFilter(ctx context.Context, client client.Client, namespace string, resources []kubearchivev1alpha1.KubeArchiveConfigResource) (*kubearchivev1alpha1.SinkFilter, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileSinkFilter")

	sf := &kubearchivev1alpha1.SinkFilter{}
	err := client.Get(ctx, types.NamespacedName{Name: constants.SinkFilterResourceName, Namespace: constants.KubeArchiveNamespace}, sf)
	if errors.IsNotFound(err) {
		sf = desiredSinkFilter(ctx, nil, namespace, resources)
		err = client.Create(ctx, sf)
		if err != nil {
			log.Error(err, "Failed to create SinkFilter "+constants.SinkFilterResourceName)
			return nil, err
		}
		return sf, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile SinkFilter "+constants.SinkFilterResourceName)
		return nil, err
	}

	sf = desiredSinkFilter(ctx, sf, namespace, resources)
	err = client.Update(ctx, sf)
	if err != nil {
		log.Error(err, "Failed to update SinkFilter "+constants.SinkFilterResourceName)
		return nil, err
	}
	return sf, nil
}

func desiredSinkFilter(ctx context.Context, sf *kubearchivev1alpha1.SinkFilter, namespace string, resources []kubearchivev1alpha1.KubeArchiveConfigResource) *kubearchivev1alpha1.SinkFilter {
	log := log.FromContext(ctx)

	log.Info("in desiredSinkFilter")

	if sf == nil {
		sf = &kubearchivev1alpha1.SinkFilter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.SinkFilterResourceName,
				Namespace: constants.KubeArchiveNamespace,
			},
			Spec: kubearchivev1alpha1.SinkFilterSpec{
				Namespaces: map[string][]kubearchivev1alpha1.KubeArchiveConfigResource{},
			},
		}
	}

	if sf.Spec.Namespaces == nil {
		sf.Spec.Namespaces = make(map[string][]kubearchivev1alpha1.KubeArchiveConfigResource)
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

func convertToYamlString(resources []kubearchivev1alpha1.KubeArchiveConfigResource) (string, error) {
	jsonBytes, err := json.Marshal(resources)
	if err != nil {
		return "", err
	}

	var data interface{}
	err = json.Unmarshal(jsonBytes, &data)
	if err != nil {
		return "", err
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

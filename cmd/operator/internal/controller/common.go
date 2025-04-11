// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// a13e => shorthand for ApiServerSource
// k9e  => shorthand for KubeArchive

import (
	"context"
	"encoding/json"
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
)

var (
	k9eNs         = os.Getenv("KUBEARCHIVE_NAMESPACE")
	a13eName      = k9eNs + "-a13e"
	k9eSinkName   = "kubearchive-sink"
	k9eBrokerName = "kubearchive-broker"
)

func reconcileA13eServiceAccount(ctx context.Context, client client.Client) error {
	log := log.FromContext(ctx)

	log.Info("in ReconcileServiceAccount")
	sa := &corev1.ServiceAccount{}

	err := client.Get(ctx, types.NamespacedName{Name: a13eName, Namespace: k9eNs}, sa)
	if errors.IsNotFound(err) {
		err = client.Create(ctx, desiredA13eServiceAccount())
		if err != nil {
			log.Error(err, "Failed to create ServiceAccount")
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to reconcile ServiceAccount")
		return err
	}

	return nil
}

func desiredA13eServiceAccount() *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a13eName,
			Namespace: k9eNs,
		},
	}
	return sa
}

func reconcileA13eRole(ctx context.Context, client client.Client, mapper meta.RESTMapper, resources []sourcesv1.APIVersionKindSelector) (*rbacv1.ClusterRole, error) {
	return reconcileClusterRole(ctx, client, a13eName, createPolicyRules(ctx, mapper, resources, []string{"get", "list", "watch"}))
}

func reconcileClusterRole(ctx context.Context, client client.Client, roleName string, rules []rbacv1.PolicyRule) (*rbacv1.ClusterRole, error) {
	log := log.FromContext(ctx)

	log.Info("in ReconcileClusterRole " + roleName)

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

	log.Info("in ReconcileApiServerSource")
	desired := desiredA13e(resources)

	existing := &sourcesv1.ApiServerSource{}
	err := client.Get(ctx, types.NamespacedName{Name: a13eName, Namespace: k9eNs}, existing)
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
		resources = append(resources, sourcesv1.APIVersionKindSelector{Kind: "ClusterKubeArchiveConfig", APIVersion: "kubearchive.org/v1alpha1"})
	}

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

func parseConfigMap(ctx context.Context, cm *corev1.ConfigMap) []sourcesv1.APIVersionKindSelector {
	log := log.FromContext(ctx)

	resourceMap := map[string]sourcesv1.APIVersionKindSelector{}
	resourceKeys := make([]string, 0)
	for namespace, yaml := range cm.Data {
		kars, err := kubearchivev1alpha1.LoadKubeArchiveConfigFromString(yaml)
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

func reconcileFilterConfigMap(ctx context.Context, client client.Client, namespace string, data string) (*corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	log.Info("in reconcileFilterConfigMap")

	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: constants.SinkFiltersConfigMapName, Namespace: k9eNs}, cm)
	if errors.IsNotFound(err) {
		cm = desiredFilterConfigMap(ctx, nil, namespace, data)
		err = client.Create(ctx, cm)
		if err != nil {
			log.Error(err, "Failed to create filter ConfigMap "+constants.SinkFiltersConfigMapName)
			return nil, err
		}
		return cm, nil
	} else if err != nil {
		log.Error(err, "Failed to reconcile filter ConfigMap "+constants.SinkFiltersConfigMapName)
		return nil, err
	}

	cm = desiredFilterConfigMap(ctx, cm, namespace, data)
	err = client.Update(ctx, cm)
	if err != nil {
		log.Error(err, "Failed to update filter ConfigMap "+constants.SinkFiltersConfigMapName)
		return nil, err
	}
	return cm, nil
}

func desiredFilterConfigMap(ctx context.Context, cm *corev1.ConfigMap, namespace string, data string) *corev1.ConfigMap {
	log := log.FromContext(ctx)

	log.Info("in desiredFilterConfigMap")

	if cm == nil {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.SinkFiltersConfigMapName,
				Namespace: k9eNs,
			},
			Data: map[string]string{},
		}
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	cm.Data[namespace] = data

	// Note that the owner reference is NOT set on the ConfigMap.  It should not be deleted when
	// the KubeArchiveConfig object is deleted.
	return cm
}

func deleteNamespaceFromFilterConfigMap(ctx context.Context, client client.Client, namespace string) error {
	log := log.FromContext(ctx)

	log.Info("in deleteNamespaceFromFilterConfigMap")

	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: constants.SinkFiltersConfigMapName, Namespace: k9eNs}, cm)
	if err != nil {
		log.Error(err, "Failed to get filter ConfigMap "+constants.SinkFiltersConfigMapName)
		return err
	}

	delete(cm.Data, namespace)
	err = client.Update(ctx, cm)
	if err != nil {
		log.Error(err, "Failed to remove namespace '"+namespace+"' from filter ConfigMap "+constants.SinkFiltersConfigMapName)
		return err
	}
	return nil
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

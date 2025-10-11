// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

const (
	resourceFinalizerName = "kubearchive.org/finalizer"
)

func desiredClusterRole(name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
	return role
}

func desiredClusterRoleBinding(name string, kind string, subjects ...rbacv1.Subject) *rbacv1.ClusterRoleBinding {
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

func createPolicyRules(ctx context.Context, mapper meta.RESTMapper, resources []kubearchivev1.APIVersionKind, verbs []string) []rbacv1.PolicyRule {
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

func desiredServiceAccount(namespace string, name string) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return sa
}

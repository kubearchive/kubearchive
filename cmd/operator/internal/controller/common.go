// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
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

func equalPolicyRules(a, b []rbacv1.PolicyRule) bool {
	if len(a) != len(b) {
		return false
	}

	// Create sorted copies of both slices
	aCopy := make([]rbacv1.PolicyRule, len(a))
	bCopy := make([]rbacv1.PolicyRule, len(b))
	copy(aCopy, a)
	copy(bCopy, b)

	// Sort the individual string slices within each PolicyRule and then sort the PolicyRules
	for i := range aCopy {
		slices.Sort(aCopy[i].APIGroups)
		slices.Sort(aCopy[i].Resources)
		slices.Sort(aCopy[i].Verbs)
	}
	for i := range bCopy {
		slices.Sort(bCopy[i].APIGroups)
		slices.Sort(bCopy[i].Resources)
		slices.Sort(bCopy[i].Verbs)
	}

	// Sort both copies before comparison
	slices.SortFunc(aCopy, comparePolicyRules)
	slices.SortFunc(bCopy, comparePolicyRules)

	for i := range aCopy {
		if !slices.Equal(aCopy[i].APIGroups, bCopy[i].APIGroups) ||
			!slices.Equal(aCopy[i].Resources, bCopy[i].Resources) ||
			!slices.Equal(aCopy[i].Verbs, bCopy[i].Verbs) {
			return false
		}
	}
	return true
}

func comparePolicyRules(a, b rbacv1.PolicyRule) int {
	if cmp := slices.Compare(a.APIGroups, b.APIGroups); cmp != 0 {
		return cmp
	}
	if cmp := slices.Compare(a.Resources, b.Resources); cmp != 0 {
		return cmp
	}
	return slices.Compare(a.Verbs, b.Verbs)
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

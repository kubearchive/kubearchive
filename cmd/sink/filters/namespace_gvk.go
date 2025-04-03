// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespaceGroupVersionKind can be represented with the following format: <namespace>:<GroupVersionKind>
type NamespaceGroupVersionKind string

// NamespaceGVKFromObject returns a NamespaceGroupVersionKind that is based on the namespace and GroupVersionKind of
// obj.
func NamespaceGVKFromObject(obj *unstructured.Unstructured) NamespaceGroupVersionKind {
	if obj == nil {
		return ":"
	}
	return NamespaceGVKFromNamespaceAndGvk(obj.GetNamespace(), obj.GetObjectKind().GroupVersionKind())
}

// GlobalNGVKFromObject returns a NamespaceGroupVersionKind that is based on the kubearchive namespace and the
// GroupVersionKind of obj.
func GlobalNGVKFromObject(obj *unstructured.Unstructured) NamespaceGroupVersionKind {
	if obj == nil {
		return ":"
	}
	return NamespaceGVKFromNamespaceAndGvk(globalKey, obj.GetObjectKind().GroupVersionKind())
}

// NamespaceGroupVersionKind returns a NamespaceGroupVersionKind from a namespace and GroupVersionKind.
func NamespaceGVKFromNamespaceAndGvk(namespace string, gvk schema.GroupVersionKind) NamespaceGroupVersionKind {
	return NamespaceGroupVersionKind(fmt.Sprintf("%s:%s", namespace, gvk))
}

// Namespace returns the namespace of a NamespaceGroupVersionKind
func (ngvk NamespaceGroupVersionKind) Namespace() string {
	split := strings.Split(ngvk.String(), ":")
	if len(split) != 2 {
		return ""
	}
	return split[0]
}

// GroupVersionKind returns the GroupVersionKind of a NamespaceGroupVersionKind as a string.
func (ngvk NamespaceGroupVersionKind) GroupVersionKind() string {
	split := strings.Split(ngvk.String(), ":")
	if len(split) != 2 {
		return ""
	}
	return split[1]
}

// String implements Stringer interface.
func (ngvk NamespaceGroupVersionKind) String() string {
	return string(ngvk)
}

// NamespaceMatcher complies with the delete function for maps.DeleteFunc.
type NamespaceMatcher func(NamespaceGroupVersionKind, cel.Program) bool

// NamespaceMatcherFromNamespace returns a NamespaceMatcher that returns true when the NamespaceGroupVersionKind passed
// in has the same namespace as namespace.
func NamespaceMatcherFromNamespace(namespace string) NamespaceMatcher {
	return func(ngvk NamespaceGroupVersionKind, _ cel.Program) bool {
		return ngvk.Namespace() == namespace
	}
}

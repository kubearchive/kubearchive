// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetLastUpdateTs returns the last time obj was updated on the cluster
func GetLastUpdateTs(obj *unstructured.Unstructured) time.Time {
	fields := obj.GetManagedFields()
	var latest *metav1.Time
	for _, elem := range fields {
		if latest == nil || latest.Before(elem.Time) {
			latest = elem.Time
		}
	}
	return latest.Time
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestSetSinkReplicas(t *testing.T) {
	resource := schema.GroupResource{Group: "apps", Resource: "deployments"}
	for _, tc := range []struct {
		name     string
		firstErr error
		attempts int
		wantErr  bool
	}{
		{
			name:     "retries conflicts with a fresh resource version",
			firstErr: apierrors.NewConflict(resource, "kubearchive-sink", errors.New("deployment changed")),
			attempts: 2,
		},
		{
			name:     "returns non-conflict errors without retrying",
			firstErr: apierrors.NewForbidden(resource, "kubearchive-sink", errors.New("access denied")),
			attempts: 1,
			wantErr:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewClientset()
			gets, updates := 0, 0
			clientset.PrependReactor("get", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
				require.Equal(t, "scale", action.GetSubresource())
				gets++
				return true, &autoscalingv1.Scale{
					ObjectMeta: metav1.ObjectMeta{ResourceVersion: strconv.Itoa(gets)},
				}, nil
			})
			clientset.PrependReactor("update", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
				require.Equal(t, "scale", action.GetSubresource())
				updates++
				scale := action.(clienttesting.UpdateAction).GetObject().(*autoscalingv1.Scale)
				require.Equal(t, int32(1), scale.Spec.Replicas)
				require.Equal(t, strconv.Itoa(updates), scale.ResourceVersion)
				if updates == 1 {
					return true, nil, tc.firstErr
				}
				return true, scale, nil
			})

			err := setSinkReplicas(clientset, 1)
			if tc.wantErr {
				require.ErrorIs(t, err, tc.firstErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.attempts, gets)
			require.Equal(t, tc.attempts, updates)
		})
	}
}

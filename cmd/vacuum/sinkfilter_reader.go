// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/filters"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

type SinkFilterReader struct {
	dynamicClient dynamic.Interface
}

func NewSinkFilterReader() (*SinkFilterReader, error) {
	client, err := k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get dynamic client: %v", err)
	}

	return &SinkFilterReader{
		dynamicClient: client,
	}, nil
}

func (r *SinkFilterReader) GetSinkFilter(ctx context.Context) (*kubearchiveapi.SinkFilter, error) {
	obj, err := r.dynamicClient.Resource(kubearchiveapi.SinkFilterGVR).
		Namespace(constants.KubeArchiveNamespace).
		Get(ctx, constants.SinkFilterResourceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("failed to get SinkFilter: %v", err)
	}

	sinkFilter, err := kubearchiveapi.ConvertUnstructuredToSinkFilter(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to SinkFilter: %v", err)
	}

	return sinkFilter, nil
}

func (r *SinkFilterReader) ProcessAllNamespaces(ctx context.Context) (map[string]map[string]filters.CelExpressions, error) {
	return r.ProcessAllNamespacesWithFilterType(ctx, filters.Vacuum)
}

func (r *SinkFilterReader) ProcessAllNamespacesWithFilterType(ctx context.Context, filterType filters.FilterType) (map[string]map[string]filters.CelExpressions, error) {
	sinkFilter, err := r.GetSinkFilter(ctx)
	if err != nil {
		return nil, err
	}

	if sinkFilter == nil {
		return map[string]map[string]filters.CelExpressions{}, nil
	}

	return filters.ExtractNamespacesByKind(sinkFilter, filterType), nil
}

func (r *SinkFilterReader) ProcessSingleNamespace(ctx context.Context, targetNamespace string) (map[string]map[string]filters.CelExpressions, error) {
	return r.ProcessSingleNamespaceWithFilterType(ctx, targetNamespace, filters.Vacuum)
}

func (r *SinkFilterReader) ProcessSingleNamespaceWithFilterType(ctx context.Context, targetNamespace string, filterType filters.FilterType) (map[string]map[string]filters.CelExpressions, error) {
	sinkFilter, err := r.GetSinkFilter(ctx)
	if err != nil {
		return nil, err
	}

	if sinkFilter == nil {
		return map[string]map[string]filters.CelExpressions{}, nil
	}

	return filters.ExtractNamespaceByKind(sinkFilter, targetNamespace, filterType), nil
}

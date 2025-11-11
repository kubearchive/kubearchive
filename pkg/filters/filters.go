// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	kcel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CelExpressions struct {
	ArchiveWhen     *cel.Program
	DeleteWhen      *cel.Program
	ArchiveOnDelete *cel.Program
}

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

func (r *SinkFilterReader) GetSinkFilter(ctx context.Context) (*kubearchivev1.SinkFilter, error) {
	obj, err := r.dynamicClient.Resource(kubearchivev1.SinkFilterGVR).
		Namespace(constants.KubeArchiveNamespace).
		Get(ctx, constants.SinkFilterResourceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("failed to get SinkFilter: %v", err)
	}

	sinkFilter, err := kubearchivev1.ConvertUnstructuredToSinkFilter(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to SinkFilter: %v", err)
	}

	return sinkFilter, nil
}

func (r *SinkFilterReader) ProcessAllNamespaces(ctx context.Context) (map[string]map[string]CelExpressions, error) {
	sinkFilter, err := r.GetSinkFilter(ctx)
	if err != nil {
		return nil, err
	}

	if sinkFilter == nil {
		return map[string]map[string]CelExpressions{}, nil
	}

	return ExtractNamespacesByKind(sinkFilter), nil
}

func (r *SinkFilterReader) ProcessSingleNamespace(ctx context.Context, targetNamespace string) (map[string]map[string]CelExpressions, error) {
	sinkFilter, err := r.GetSinkFilter(ctx)
	if err != nil {
		return nil, err
	}

	if sinkFilter == nil {
		return map[string]map[string]CelExpressions{}, nil
	}

	return ExtractNamespaceByKind(sinkFilter, targetNamespace), nil
}

func ExtractClusterCELExpressionsByKind(sinkFilter *kubearchivev1.SinkFilter) map[string]CelExpressions {
	expressionsByKind := make(map[string]CelExpressions)

	if len(sinkFilter.Spec.Cluster) == 0 {
		return expressionsByKind
	}

	for _, res := range sinkFilter.Spec.Cluster {
		key := res.Selector.Kind + "-" + res.Selector.APIVersion

		celExpr := CelExpressions{
			ArchiveWhen:     CompileCELExpression(res.ArchiveWhen, "ArchiveWhen", "ckac"),
			DeleteWhen:      CompileCELExpression(res.DeleteWhen, "DeleteWhen", "ckac"),
			ArchiveOnDelete: CompileCELExpression(res.ArchiveOnDelete, "ArchiveOnDelete", "ckac"),
		}
		expressionsByKind[key] = celExpr
	}

	return expressionsByKind
}

func ExtractNamespacesByKind(sinkFilter *kubearchivev1.SinkFilter) map[string]map[string]CelExpressions {
	var namespacesToProcess []string
	for ns := range sinkFilter.Spec.Namespaces {
		namespacesToProcess = append(namespacesToProcess, ns)
	}

	return extractNamespacesByKindsList(sinkFilter, namespacesToProcess)
}

func ExtractNamespaceByKind(sinkFilter *kubearchivev1.SinkFilter, namespace string) map[string]map[string]CelExpressions {
	return extractNamespacesByKindsList(sinkFilter, []string{namespace})
}

func extractNamespacesByKindsList(sinkFilter *kubearchivev1.SinkFilter, namespacesToProcess []string) map[string]map[string]CelExpressions {
	namespacesByKinds := make(map[string]map[string]CelExpressions)

	for _, ns := range namespacesToProcess {
		resources, exists := sinkFilter.Spec.Namespaces[ns]
		if !exists {
			continue
		}

		for _, res := range resources {
			key := res.Selector.Key()

			celExpr := CelExpressions{
				ArchiveWhen:     CompileCELExpression(res.ArchiveWhen, "ArchiveWhen", ns),
				DeleteWhen:      CompileCELExpression(res.DeleteWhen, "DeleteWhen", ns),
				ArchiveOnDelete: CompileCELExpression(res.ArchiveOnDelete, "ArchiveOnDelete", ns),
			}

			if namespaces, exists := namespacesByKinds[key]; exists {
				namespaces[ns] = celExpr
			} else {
				namespacesByKinds[key] = map[string]CelExpressions{ns: celExpr}
			}
		}
	}

	return namespacesByKinds
}

func CompileCELExpression(expression, expressionType, namespace string) *cel.Program {
	if expression == "" {
		return nil
	}

	compiled, err := kcel.CompileCELExpr(expression)
	if err != nil {
		log.Log.Error(err, "Failed to compile CEL expression", "type", expressionType, "namespace", namespace, "expression", expression)
		return nil
	}

	return compiled
}

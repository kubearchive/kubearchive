// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"github.com/google/cel-go/cel"
	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	kcel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FilterType int

const (
	Vacuum FilterType = iota
	Controller
)

type KeepLastWhenRule struct {
	When     *cel.Program
	WhenText string
	Count    int
	Sort     string
}

type CelExpressions struct {
	ArchiveWhen     *cel.Program
	DeleteWhen      *cel.Program
	ArchiveOnDelete *cel.Program
	KeepLastWhen    []KeepLastWhenRule
}

func ExtractAllNamespacesByKinds(sinkFilter *kubearchivev1.SinkFilter, filterType FilterType) map[string]map[string]CelExpressions {
	var namespacesToProcess []string
	for ns := range sinkFilter.Spec.Namespaces {
		namespacesToProcess = append(namespacesToProcess, ns)
	}

	return extractNamespacesByKindsList(sinkFilter, namespacesToProcess, filterType)
}

func ExtractSingleNamespaceByKinds(sinkFilter *kubearchivev1.SinkFilter, targetNamespace string, filterType FilterType) map[string]map[string]CelExpressions {
	namespacesToProcess := []string{targetNamespace, constants.SinkFilterGlobalNamespace}

	return extractNamespacesByKindsList(sinkFilter, namespacesToProcess, filterType)
}

func extractNamespacesByKindsList(sinkFilter *kubearchivev1.SinkFilter, namespacesToProcess []string, filterType FilterType) map[string]map[string]CelExpressions {
	namespacesByKinds := make(map[string]map[string]CelExpressions)

	for _, ns := range namespacesToProcess {
		resources, exists := sinkFilter.Spec.Namespaces[ns]
		if !exists {
			continue
		}

		for _, res := range resources {
			key := res.Selector.Kind + "-" + res.Selector.APIVersion

			celExpr := CelExpressions{
				ArchiveWhen: CompileCELExpression(res.ArchiveWhen, "ArchiveWhen", ns),
			}

			// Compile different expressions based on filter type
			switch filterType {
			case Vacuum:
				// For vacuum: compile deleteWhen and keepLastWhen
				celExpr.DeleteWhen = CompileCELExpression(res.DeleteWhen, "DeleteWhen", ns)
				celExpr.KeepLastWhen = compileKeepLastWhenRules(res.KeepLastWhen, ns)
			case Controller:
				// For controller: compile deleteWhen and archiveOnDelete
				celExpr.DeleteWhen = CompileCELExpression(res.DeleteWhen, "DeleteWhen", ns)
				celExpr.ArchiveOnDelete = CompileCELExpression(res.ArchiveOnDelete, "ArchiveOnDelete", ns)
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

func compileKeepLastWhenRules(keepLastRules []kubearchivev1.KeepLastRule, namespace string) []KeepLastWhenRule {
	var compiledRules []KeepLastWhenRule

	for _, rule := range keepLastRules {
		compiledRule := KeepLastWhenRule{
			When:     CompileCELExpression(rule.When, "KeepLastWhen.When", namespace),
			WhenText: rule.When,
			Count:    rule.Count,
			Sort:     rule.Sort,
		}
		compiledRules = append(compiledRules, compiledRule)
	}

	return compiledRules
}

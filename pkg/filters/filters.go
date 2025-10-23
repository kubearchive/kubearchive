// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"strings"

	"github.com/google/cel-go/cel"
	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	kcel "github.com/kubearchive/kubearchive/pkg/cel"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FilterType int

const (
	Vacuum FilterType = iota
	Controller
)

type KeepLastWhenRule struct {
	Name     string
	When     *cel.Program
	WhenText string
	Count    int
	SortBy   string
}

type CelExpressions struct {
	ArchiveWhen     *cel.Program
	DeleteWhen      *cel.Program
	ArchiveOnDelete *cel.Program
	KeepLastWhen    []KeepLastWhenRule
}

func ExtractClusterCELExpressionsByKind(sinkFilter *kubearchivev1.SinkFilter, filterType FilterType) map[string]CelExpressions {
	expressionsByKind := make(map[string]CelExpressions)

	if len(sinkFilter.Spec.Cluster) == 0 {
		return expressionsByKind
	}

	for _, res := range sinkFilter.Spec.Cluster {
		key := res.Selector.Kind + "-" + res.Selector.APIVersion

		celExpr := CelExpressions{
			ArchiveWhen: CompileCELExpression(res.ArchiveWhen, "ArchiveWhen", "ckac"),
		}

		// Compile different expressions based on filter type
		switch filterType {
		case Vacuum:
			// For vacuum: compile keepLastWhen only
			celExpr.KeepLastWhen = compileClusterKeepLastWhenRules(res.KeepLastWhen, "ckac")
		case Controller:
			// For controller: compile deleteWhen and archiveOnDelete only
			celExpr.DeleteWhen = CompileCELExpression(res.DeleteWhen, "DeleteWhen", "ckac")
			celExpr.ArchiveOnDelete = CompileCELExpression(res.ArchiveOnDelete, "ArchiveOnDelete", "ckac")
		}

		expressionsByKind[key] = celExpr
	}

	return expressionsByKind
}

func ExtractNamespacesByKind(sinkFilter *kubearchivev1.SinkFilter, filterType FilterType) map[string]map[string]CelExpressions {
	var namespacesToProcess []string
	for ns := range sinkFilter.Spec.Namespaces {
		namespacesToProcess = append(namespacesToProcess, ns)
	}

	return extractNamespacesByKindsList(sinkFilter, namespacesToProcess, filterType)
}

func ExtractNamespaceByKind(sinkFilter *kubearchivev1.SinkFilter, namespace string, filterType FilterType) map[string]map[string]CelExpressions {
	return extractNamespacesByKindsList(sinkFilter, []string{namespace}, filterType)
}

func extractNamespacesByKindsList(sinkFilter *kubearchivev1.SinkFilter, namespacesToProcess []string, filterType FilterType) map[string]map[string]CelExpressions {
	namespacesByKinds := make(map[string]map[string]CelExpressions)

	for _, ns := range namespacesToProcess {
		resources, exists := sinkFilter.Spec.Namespaces[ns]
		if !exists {
			continue
		}

		for _, res := range resources {
			key := res.Selector.Key()

			celExpr := CelExpressions{
				ArchiveWhen: CompileCELExpression(res.ArchiveWhen, "ArchiveWhen", ns),
			}

			// Compile different expressions based on filter type
			switch filterType {
			case Vacuum:
				// For vacuum: compile keepLastWhen only
				celExpr.KeepLastWhen = compileKeepLastWhenRules(res.KeepLastWhen, ns)
			case Controller:
				// For controller: compile deleteWhen and archiveOnDelete only
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

func compileKeepLastWhenRules(keepLastWhen *kubearchivev1.KeepLastWhenConfig, namespace string) []KeepLastWhenRule {
	var compiledRules []KeepLastWhenRule

	if keepLastWhen == nil {
		return compiledRules
	}

	for _, rule := range keepLastWhen.Keep {
		compiledRule := KeepLastWhenRule{
			When:     CompileCELExpression(rule.When, "KeepLastWhen.Keep.When", namespace),
			WhenText: strings.TrimSpace(rule.When),
			Count:    rule.Count,
			SortBy:   rule.SortBy,
		}
		compiledRules = append(compiledRules, compiledRule)
	}

	for _, rule := range keepLastWhen.Override {
		compiledRule := KeepLastWhenRule{
			Name:  rule.Name,
			Count: rule.Count,
		}
		compiledRules = append(compiledRules, compiledRule)
	}

	return compiledRules
}

func compileClusterKeepLastWhenRules(keepLastWhen []kubearchivev1.ClusterKeepLastRule, namespace string) []KeepLastWhenRule {
	var compiledRules []KeepLastWhenRule

	if len(keepLastWhen) == 0 {
		return compiledRules
	}

	for _, rule := range keepLastWhen {
		compiledRule := KeepLastWhenRule{
			Name:     rule.Name,
			When:     CompileCELExpression(rule.When, "KeepLastWhen.When", namespace),
			WhenText: strings.TrimSpace(rule.When),
			Count:    rule.Count,
			SortBy:   rule.SortBy,
		}
		compiledRules = append(compiledRules, compiledRule)
	}

	return compiledRules
}

// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package cel

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	celext "github.com/google/cel-go/ext"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// CompileOrCELExpression attempts to compile cel expression strings into a cel.Program. If exprs contains more than one cel
// expression string, it will surround each cell expression in parentheses and or them together.
func CompileOrCELExpression(exprs ...string) (*cel.Program, error) {
	exprs = FormatCelExprs(exprs...)
	expr := strings.Join(exprs, " || ")
	return CompileCELExpr(expr)
}

func CompileCELExpr(expr string) (*cel.Program, error) {
	mapStrDyn := types.NewMapType(types.StringType, types.DynType)
	env, err := cel.NewEnv(
		celext.Strings(),
		celext.Encoders(),
		celext.Sets(),
		celext.Lists(),
		celext.Math(),
		cel.VariableDecls(
			decls.NewVariable("metadata", mapStrDyn),
			decls.NewVariable("spec", mapStrDyn),
			decls.NewVariable("status", mapStrDyn),
		),
	)
	if err != nil {
		return nil, err
	}
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	program, err := env.Program(checked)
	if err != nil {
		return nil, err
	}
	return &program, err
}

// FormatCelExprs takes a []string of cel expression strings and surrounds them with parentheses so that the can be
// combined into a larger expression. Any empty strings in exprs, are removed from the []string returned.
func FormatCelExprs(exprs ...string) []string {
	newExprs := []string{}
	if len(exprs) < 2 {
		return exprs
	}
	for _, expr := range exprs {
		if expr != "" {
			newExprs = append(newExprs, fmt.Sprintf("( %s )", expr))
		}
	}
	return newExprs
}

// ExecuteBoolean CEL executes the cel program with obj as the input. If the program returns and error or
// if the program returns a value that cannot be converted to bool, false is returned. Otherwise the bool
// returned by the CEL program is returned.
func ExecuteBooleanCEL(ctx context.Context, program cel.Program, obj *unstructured.Unstructured) bool {
	val := ExecuteCEL(ctx, program, obj)
	if val == nil {
		return false
	}
	boolVal, ok := val.Value().(bool)
	return ok && boolVal
}

func ExecuteCEL(ctx context.Context, program cel.Program, obj *unstructured.Unstructured) ref.Val {
	val, _, _ := program.ContextEval(ctx, obj.Object)
	return val
}

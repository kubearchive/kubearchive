// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	celext "github.com/google/cel-go/ext"
	"go.opentelemetry.io/otel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var mapStrDyn = types.NewMapType(types.StringType, types.DynType)
var env *cel.Env

func init() {
	var err error
	env, err = cel.NewEnv(
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
		cel.Function("now",
			cel.Overload("now",
				[]*cel.Type{},
				cel.TimestampType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					return types.Timestamp{Time: time.Now()}
				},
				),
			),
		),
		cel.OptionalTypes(),
	)
	if err != nil {
		panic(fmt.Sprintf("Error creating CEL environment: %s", err.Error()))
	}
}

func CompileCELExpr(expr string) (*cel.Program, error) {
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

// ExecuteBoolean CEL executes the cel program with obj as the input. If the program returns and error or
// if the program returns a value that cannot be converted to bool, false is returned. Otherwise the bool
// returned by the CEL program is returned.
func ExecuteBooleanCEL(ctx context.Context, program cel.Program, obj *unstructured.Unstructured) bool {
	val, _ := ExecuteCEL(ctx, program, obj)
	if val == nil {
		return false
	}
	boolVal, ok := val.Value().(bool)
	return ok && boolVal
}

func ExecuteCEL(ctx context.Context, program cel.Program, obj *unstructured.Unstructured) (ref.Val, error) {
	tracer := otel.Tracer("kubearchive")
	ctx, span := tracer.Start(ctx, "ExecuteCEL")
	defer span.End()

	val, _, err := program.ContextEval(ctx, obj.Object)
	return val, err
}

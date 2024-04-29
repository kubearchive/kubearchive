// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
)

func Start() error {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return err
	}

	res, err := resource.New(
		context.Background(),
		resource.WithTelemetrySDK(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return err
	}

	tp := sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(exporter),
		sdkTrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return nil
}

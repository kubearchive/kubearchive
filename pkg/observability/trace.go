// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
)

var tp *sdkTrace.TracerProvider

// Start creates a Span Processor and exporter, registers them with a TracerProvider, and sets the default
// TracerProvider and SetTextMapPropagator
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

	tp = sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(exporter),
		sdkTrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return nil
}

// FlushSpanBuffer exports all completed spans that have not been exported for all SpanProcessors registered with the
// global TracerProvider. If the provided context has a timeout or a deadline, it will be respected.
func FlushSpanBuffer(ctx context.Context) error {
	if tp != nil {
		return tp.ForceFlush(ctx)
	}

	return fmt.Errorf("Cannot flush spans. No TracerProvider has been configured.")
}

// Shutdown shuts down the TracerProvider, any SpanProcessors that have been registered, and exporters associated with
// the SpanProcessors. This should only be called after all spans have been ended. After this function is called, spans
// cannot be created, ended or modified.
func Shutdown(ctx context.Context) error {
	if tp != nil {
		err := FlushSpanBuffer(ctx)
		if err != nil {
			return err
		}
		return tp.Shutdown(ctx)
	}

	return fmt.Errorf("Cannot shutdown TracerProvider. None have been started")
}

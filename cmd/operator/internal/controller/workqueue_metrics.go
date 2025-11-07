// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
// Code required to comply with the interfaces the workqueue
// package defines for instrumentation. The native implementation
// is for Prometheus, so here we coded a series of adapters to
// accomodate our Opentelemetry instrumentation
package controller

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"k8s.io/client-go/util/workqueue"
)

type OpenTelemetryWorkqueueMetricsProvider struct{}

type Gauge struct {
	UpDownCounter metric.Int64UpDownCounter
	name          string
}

func (g Gauge) Inc() {
	g.UpDownCounter.Add(
		context.Background(),
		1,
		metric.WithAttributes(attribute.String("queue", g.name)),
	)
}

func (g Gauge) Dec() {
	g.UpDownCounter.Add(
		context.Background(),
		-1,
		metric.WithAttributes(attribute.String("queue", g.name)),
	)
}

type Counter struct {
	Counter metric.Int64Counter
	name    string
}

func (c Counter) Inc() {
	c.Counter.Add(
		context.Background(),
		1,
		metric.WithAttributes(attribute.String("queue", c.name)),
	)
}

type HistogramMetric struct {
	Histogram metric.Float64Histogram
	name      string
}

func (h HistogramMetric) Observe(value float64) {
	h.Histogram.Record(
		context.Background(),
		value,
		metric.WithAttributes(attribute.String("queue", h.name)),
	)
}

type SettableGauge struct {
	Gauge metric.Float64Gauge
	name  string
}

func (s SettableGauge) Set(value float64) {
	s.Gauge.Record(
		context.Background(),
		value,
		metric.WithAttributes(attribute.String("queue", s.name)),
	)
}

func (OpenTelemetryWorkqueueMetricsProvider) NewDepthMetric(name string) workqueue.GaugeMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	upDownCounter, err := meter.Int64UpDownCounter(
		"workqueue.depth",
		metric.WithDescription("Current number of items in the workqueue"),
	)
	if err != nil {
		panic(err)
	}
	return Gauge{UpDownCounter: upDownCounter, name: name}
}

func (OpenTelemetryWorkqueueMetricsProvider) NewAddsMetric(name string) workqueue.CounterMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	counter, err := meter.Int64Counter(
		"workqueue.adds",
		metric.WithDescription("Total number of adds handled by the workqueue"),
	)
	if err != nil {
		panic(err)
	}
	return Counter{Counter: counter, name: name}
}

func (OpenTelemetryWorkqueueMetricsProvider) NewLatencyMetric(name string) workqueue.HistogramMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	histogram, err := meter.Float64Histogram(
		"workqueue.latency",
		metric.WithDescription("How long an item stays in the workqueue before being requested"),
		metric.WithUnit("second"),
	)
	if err != nil {
		panic(err)
	}
	return HistogramMetric{Histogram: histogram, name: name}
}

func (OpenTelemetryWorkqueueMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	histogram, err := meter.Float64Histogram(
		"workqueue.duration",
		metric.WithDescription("How long processing an item from the workqueue"),
		metric.WithUnit("second"),
	)
	if err != nil {
		panic(err)
	}
	return HistogramMetric{Histogram: histogram, name: name}
}

func (OpenTelemetryWorkqueueMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	gauge, err := meter.Float64Gauge(
		"workqueue.unifinished_work",
		metric.WithDescription("Sum of all the active tasks duration"),
		metric.WithUnit("second"),
	)
	if err != nil {
		panic(err)
	}
	return SettableGauge{Gauge: gauge, name: name}
}

func (OpenTelemetryWorkqueueMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	gauge, err := meter.Float64Gauge(
		"workqueue.longest_running",
		metric.WithDescription("How long the oldest task has been running for"),
		metric.WithUnit("second"),
	)
	if err != nil {
		panic(err)
	}
	return SettableGauge{Gauge: gauge, name: name}
}

func (OpenTelemetryWorkqueueMetricsProvider) NewRetriesMetric(name string) workqueue.CounterMetric {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	counter, err := meter.Int64Counter(
		"workqueue.retries",
		metric.WithDescription("Total number of retries handled by the workqueue"),
	)
	if err != nil {
		panic(err)
	}
	return Counter{Counter: counter, name: name}
}

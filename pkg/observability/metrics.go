// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var ArchivedResources metric.Int64UpDownCounter

func init() {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	var err error
	ArchivedResources, err = meter.Int64UpDownCounter(
		"kubearchive.archived",
		metric.WithDescription("The number of resources archived by KubeArchive"),
		metric.WithUnit("{count}"))
	if err != nil {
		panic(err)
	}
}

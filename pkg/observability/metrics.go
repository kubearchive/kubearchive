// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var CloudEvents metric.Int64UpDownCounter

type CEResult string

const (
	CEResultInsert          CEResult = "insert"
	CEResultUpdate          CEResult = "update"
	CEResultNone            CEResult = "none"
	CEResultError           CEResult = "error"
	CEResultNoMatch         CEResult = "no_match"
	CEResultNoConfiguration CEResult = "no_conf"
)

func NewCEResultFromWriteResourceResult(result interfaces.WriteResourceResult) CEResult {
	switch result {
	case interfaces.WriteResourceResultInserted:
		return CEResultInsert
	case interfaces.WriteResourceResultUpdated:
		return CEResultUpdate
	case interfaces.WriteResourceResultNone:
		return CEResultNone
	case interfaces.WriteResourceResultError:
		return CEResultError
	default:
		return CEResultError
	}
}

func init() {
	meter := otel.Meter("github.com/kubearchive/kubearchive")
	var err error
	CloudEvents, err = meter.Int64UpDownCounter(
		"kubearchive.cloudevents.total",
		metric.WithDescription("Total number of CloudEvents received broken down by type, resource type and result of its processing"),
		metric.WithUnit("{count}"))
	if err != nil {
		panic(err)
	}
}

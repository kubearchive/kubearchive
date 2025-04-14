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
	CEResultInsert  CEResult = "insert"
	CEResultUpdate  CEResult = "update"
	CEResultNone    CEResult = "none"
	CEResultError   CEResult = "error"
	CEResultNoMatch CEResult = "no_match"
)

func NewCEResultFromWriteResourceResult(result interfaces.WriteResourceResult) CEResult {
	if result == interfaces.WriteResourceResultInserted {
		return CEResultInsert
	} else if result == interfaces.WriteResourceResultUpdated {
		return CEResultUpdate
	} else if result == interfaces.WriteResourceResultNone {
		return CEResultNone
	} else if result == interfaces.WriteResourceResultError {
		return CEResultError
	}

	return CEResultError
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

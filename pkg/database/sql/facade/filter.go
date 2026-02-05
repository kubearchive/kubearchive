// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import (
	"context"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/models"
)

// DBFilter encapsulates all the filter functions that must be implemented by the drivers
// All its functions share the same signature
type DBFilter interface {
	KindApiVersionFilter(cond sqlbuilder.Cond, kind, apiVersion string) string
	NamespaceFilter(cond sqlbuilder.Cond, ns string) string
	NameFilter(cond sqlbuilder.Cond, name string) string
	NameWildcardFilter(cond sqlbuilder.Cond, namePattern string) string
	CreationTSAndIDFilter(cond sqlbuilder.Cond, continueDate, continueId string) string
	CreationTimestampAfterFilter(cond sqlbuilder.Cond, timestamp time.Time) string
	CreationTimestampBeforeFilter(cond sqlbuilder.Cond, timestamp time.Time) string
	OwnerFilter(cond sqlbuilder.Cond, ownersUuids []string) string
	UuidsFilter(cond sqlbuilder.Cond, uuids []string) string
	UuidFilter(cond sqlbuilder.Cond, uuid string) string

	// ApplyLabelFilters applies all label filters using EXISTS/NOT EXISTS subqueries
	// This method modifies the SelectBuilder by adding WHERE conditions with correlated subqueries
	ApplyLabelFilters(ctx context.Context, querier sqlx.QueryerContext, sb *sqlbuilder.SelectBuilder, labelFilters *models.LabelFilters) error

	ContainerNameFilter(cond sqlbuilder.Cond, containerName string) string
}

// PartialDBFilterImpl implements partially the DBFilter interface
// with the default selectors with non-specific DBMS functions
type PartialDBFilterImpl struct{}

func (PartialDBFilterImpl) KindApiVersionFilter(cond sqlbuilder.Cond, kind, apiVersion string) string {
	return cond.And(cond.Equal("kind", kind), cond.Equal("api_version", apiVersion))
}

func (PartialDBFilterImpl) NamespaceFilter(cond sqlbuilder.Cond, ns string) string {
	return cond.Equal("namespace", ns)
}

func (PartialDBFilterImpl) NameFilter(cond sqlbuilder.Cond, name string) string {
	return cond.Equal("name", name)
}

func (PartialDBFilterImpl) NameWildcardFilter(cond sqlbuilder.Cond, namePattern string) string {
	return cond.Like("LOWER(name)", cond.Var(namePattern))
}

func (PartialDBFilterImpl) UuidsFilter(cond sqlbuilder.Cond, uuids []string) string {
	var parsedUuids []any
	for _, v := range uuids {
		parsedUuids = append(parsedUuids, v)
	}
	return cond.In("uuid", parsedUuids...)
}

func (PartialDBFilterImpl) UuidFilter(cond sqlbuilder.Cond, uuid string) string {
	return cond.Equal("uuid", uuid)
}

func (PartialDBFilterImpl) ContainerNameFilter(cond sqlbuilder.Cond, containerName string) string {
	return cond.Equal("container_name", containerName)
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
)

type queryPerformer[T any] struct {
	querier sqlx.QueryerContext
	flavor  sqlbuilder.Flavor
}

func newQueryPerformer[T any](querier sqlx.QueryerContext, flavor sqlbuilder.Flavor) queryPerformer[T] {
	return queryPerformer[T]{querier, flavor}
}

func (q queryPerformer[T]) performSingleRowQuery(ctx context.Context, builder sqlbuilder.Builder) (T, error) {
	var t T
	query, args := builder.BuildWithFlavor(q.flavor)
	err := sqlx.GetContext(ctx, q.querier, &t, query, args...)
	return t, err
}

func (q queryPerformer[T]) performQuery(ctx context.Context, builder sqlbuilder.Builder) ([]T, error) {
	var res []T
	query, args := builder.BuildWithFlavor(q.flavor)
	err := sqlx.SelectContext(ctx, q.querier, &res, query, args...)
	return res, err
}

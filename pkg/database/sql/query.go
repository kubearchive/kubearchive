// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	dbErrors "github.com/kubearchive/kubearchive/pkg/database/errors"
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
	return t, dbErrors.WrapQueryError(ctx, err)
}

func (q queryPerformer[T]) performQuery(ctx context.Context, builder sqlbuilder.Builder) ([]T, error) {
	var res []T
	query, args := builder.BuildWithFlavor(q.flavor)
	err := sqlx.SelectContext(ctx, q.querier, &res, query, args...)
	return res, dbErrors.WrapQueryError(ctx, err)
}

// performStreamQuery executes the query using queryCtx (e.g. one with a deadline) and
// iterates rows under iterCtx. Separating the two contexts lets callers apply a timeout
// only to the query-execution phase without aborting row iteration mid-stream.
func (q queryPerformer[T]) performStreamQuery(queryCtx, iterCtx context.Context, builder sqlbuilder.Builder, fn func(T) error) error {
	query, args := builder.BuildWithFlavor(q.flavor)
	rows, err := q.querier.QueryxContext(queryCtx, query, args...)
	if err != nil {
		return dbErrors.WrapQueryError(queryCtx, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := iterCtx.Err(); err != nil {
			return err
		}
		var t T
		if err := rows.StructScan(&t); err != nil {
			return err
		}
		if err := fn(t); err != nil {
			return err
		}
	}
	return dbErrors.WrapQueryError(queryCtx, rows.Err())
}

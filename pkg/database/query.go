// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
)

type queryPerformer[T any] struct {
	db     *sqlx.DB
	flavor sqlbuilder.Flavor
}

func newQueryPerformer[T any](db *sqlx.DB, flavor sqlbuilder.Flavor) queryPerformer[T] {
	return queryPerformer[T]{db, flavor}
}

func (q queryPerformer[T]) performSingleRowQuery(ctx context.Context, builder sqlbuilder.Builder) (T, error) {
	var t T
	query, args := builder.BuildWithFlavor(q.flavor)
	err := q.db.GetContext(ctx, &t, query, args...)
	return t, err
}

func (q queryPerformer[T]) performQuery(ctx context.Context, builder sqlbuilder.Builder) ([]T, error) {
	var res []T
	query, args := builder.BuildWithFlavor(q.flavor)
	err := q.db.SelectContext(ctx, &res, query, args...)
	return res, err
}

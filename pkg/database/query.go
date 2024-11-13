// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type paramQuery struct {
	query         string
	arguments     []any
	dbArrayParser DBParamParser
	hasArray      bool
}

func (q *paramQuery) addStringParams(args ...string) {
	for _, arg := range args {
		q.arguments = append(q.arguments, arg)
	}
}

func (q *paramQuery) addStringArrayParam(arg []string) {
	q.hasArray = true
	q.arguments = append(q.arguments, arg)
}

func (q *paramQuery) parse() (string, []any, error) {
	if !q.hasArray {
		return q.query, q.arguments, nil
	} else {
		return q.dbArrayParser.ParseParams(q.query, q.arguments...)
	}
}

type queryPerformer[T any] struct {
	db *sqlx.DB
}

func newQueryPerformer[T any](db *sqlx.DB) queryPerformer[T] {
	return queryPerformer[T]{db}
}

func (q queryPerformer[T]) performQuery(ctx context.Context, paramQuery *paramQuery) ([]T, error) {
	query, args, err := paramQuery.parse()
	if err != nil {
		return nil, err
	}
	var res []T
	err = q.db.SelectContext(ctx, &res, query, args...)
	if err != nil {
		return nil, err
	}
	return res, nil
}

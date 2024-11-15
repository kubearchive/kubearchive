// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type paramQuery struct {
	selector      Selector
	filters       []Filter
	sorter        Sorter
	limiter       Limiter
	arguments     []any
	dbArrayParser DBParamParser
	hasArray      bool
	numParams     int
}

type filterGetter func(int) (Filter, int)
type limiterGetter func(int) Limiter

func (q *paramQuery) query() string {
	query := string(q.selector)
	for i, filter := range q.filters {
		if i == 0 {
			query += " WHERE"
		} else {
			query += " AND"
		}
		query += fmt.Sprintf(" %s", filter)
	}
	if q.sorter != "" {
		query += fmt.Sprintf(" %s", q.sorter)
	}
	if q.limiter != "" {
		query += fmt.Sprintf(" %s", q.limiter)
	}
	return query
}

func (q *paramQuery) addFilters(filterGetter ...filterGetter) {
	for _, getter := range filterGetter {
		filter, numParams := getter(q.numParams + 1)
		q.numParams += numParams
		q.filters = append(q.filters, filter)
	}
}

func (q *paramQuery) setLimiter(limiterGetter limiterGetter) {
	q.limiter = limiterGetter(q.numParams + 1)
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
		return q.query(), q.arguments, nil
	} else {
		return q.dbArrayParser.ParseParams(q.query(), q.arguments...)
	}
}

type queryPerformer[T any] struct {
	db *sqlx.DB
}

func newQueryPerformer[T any](db *sqlx.DB) queryPerformer[T] {
	return queryPerformer[T]{db}
}

func (q queryPerformer[T]) performSingleRowQuery(ctx context.Context, paramQuery *paramQuery) (T, error) {
	var t T
	query, args, err := paramQuery.parse()
	if err != nil {
		return t, err
	}
	err = q.db.GetContext(ctx, &t, query, args...)
	return t, err
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

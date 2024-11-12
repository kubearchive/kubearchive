// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
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
	zeroVal T
	db      *sql.DB
}

func newQueryPerformer[T any](db *sql.DB) queryPerformer[T] {
	var zeroVal T
	return queryPerformer[T]{zeroVal, db}
}

func (q queryPerformer[T]) performQuery(ctx context.Context, paramQuery *paramQuery) ([]T, error) {
	query, args, err := paramQuery.parse()
	if err != nil {
		return nil, err
	}
	return q.parseRows(q.db.QueryContext(ctx, query, args...))
}

func (q queryPerformer[T]) parseRows(rows *sql.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err = rows.Close()
	}(rows)
	switch reflect.TypeOf(q.zeroVal).Kind() {
	case reflect.Struct:
		return q.structScan(rows)
	default:
		return q.oneFieldScan(rows)
	}
}

func (q queryPerformer[T]) oneFieldScan(rows *sql.Rows) ([]T, error) {
	var res []T
	for rows.Next() {
		var val T
		if err := rows.Scan(&val); err != nil {
			return res, err
		}
		res = append(res, val)
	}
	return res, nil
}

// structScan is a function based on
// https://ferencfbin.medium.com/golang-own-structscan-method-for-sql-rows-978c5c80f9b5
// while this feature isn't natively supported in db/sql
// https://github.com/golang/go/issues/61637
func (q queryPerformer[T]) structScan(rows *sql.Rows) ([]T, error) {
	var res []T
	v := reflect.ValueOf(&q.zeroVal)
	if v.Kind() != reflect.Ptr {
		return res, errors.New("must pass a pointer, not a value, to StructScan destination")
	}

	v = reflect.Indirect(v)
	t := v.Type()

	cols, _ := rows.Columns()

	var rowsMap []map[string]any
	for rows.Next() {
		columns := make([]any, len(cols))
		columnPointers := make([]any, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return res, err
		}

		m := make(map[string]any)
		for i, colName := range cols {
			val := columnPointers[i].(*any)
			m[colName] = *val
		}
		rowsMap = append(rowsMap, m)
	}

	for _, m := range rowsMap {
		for i := 0; i < v.NumField(); i++ {
			field := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]

			if item, ok := m[field]; ok {
				if v.Field(i).CanSet() {
					if item != nil {
						switch v.Field(i).Kind() {
						case reflect.String:
							v.Field(i).SetString(fmt.Sprintf("%s", item))
						case reflect.Int64:
							v.Field(i).SetInt(item.(int64))
						case reflect.Float32, reflect.Float64:
							v.Field(i).SetFloat(item.(float64))
						case reflect.Ptr:
							if reflect.ValueOf(item).Kind() == reflect.Bool {
								itemBool := item.(bool)
								v.Field(i).Set(reflect.ValueOf(&itemBool))
							}
						case reflect.Struct, reflect.Slice:
							v.Field(i).Set(reflect.ValueOf(item))
						default:
							fmt.Println(t.Field(i).Name, ": ", v.Field(i).Kind(), " - > - ", reflect.ValueOf(item).Kind())
						}
					}
				}
			}
		}
		res = append(res, v.Interface().(T))
	}

	return res, nil
}

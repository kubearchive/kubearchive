// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import "github.com/huandu/go-sqlbuilder"

// DBDeleter encapsulates all the deletion functions that must be implemented by the drivers
type DBDeleter interface {
	UrlDeleter() *sqlbuilder.DeleteBuilder
}

type DBDeleterImpl struct{}

func (DBDeleterImpl) UrlDeleter() *sqlbuilder.DeleteBuilder {
	db := sqlbuilder.NewDeleteBuilder()
	db.DeleteFrom("log_url")
	return db
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import (
	"database/sql"

	"github.com/huandu/go-sqlbuilder"
)

// DBInserter encapsulates all the writer functions that must be implemented by the drivers
type DBInserter interface {
	ResourceInserter(
		uuid, apiVersion, kind, name, namespace, version string,
		clusterDeletedTs sql.NullString,
		data []byte,
	) *sqlbuilder.InsertBuilder
	UrlInserter(uuid, url, containerName string) *sqlbuilder.InsertBuilder
}

type PartialDBInserterImpl struct{}

func (PartialDBInserterImpl) UrlInserter(uuid, url, containerName string) *sqlbuilder.InsertBuilder {
	ib := sqlbuilder.NewInsertBuilder()
	ib.InsertInto("log_url")
	ib.Cols("uuid", "url", "container_name")
	ib.Values(uuid, url, containerName)
	return ib
}

// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import (
	"database/sql"
	"time"

	"github.com/huandu/go-sqlbuilder"
)

// DBInserter encapsulates all the writer functions that must be implemented by the drivers
type DBInserter interface {
	ResourceInserter(
		uuid, apiVersion, kind, name, namespace, version string,
		clusterUpdatedTs time.Time,
		clusterDeletedTs sql.NullString,
		data []byte,
	) *sqlbuilder.InsertBuilder
	UrlInserter(uuid, url, containerName, jsonPath string) *sqlbuilder.InsertBuilder
}

type PartialDBInserterImpl struct{}

func (PartialDBInserterImpl) UrlInserter(uuid, url, containerName, jsonPath string) *sqlbuilder.InsertBuilder {
	ib := sqlbuilder.NewInsertBuilder()
	ib.InsertInto("log_url")
	ib.Cols("uuid", "url", "container_name", "json_path")
	ib.Values(uuid, url, containerName, jsonPath)
	return ib
}

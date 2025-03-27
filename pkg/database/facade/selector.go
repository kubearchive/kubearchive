// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import "github.com/huandu/go-sqlbuilder"

// DBSelector encapsulates all the selector functions that must be implemented by the drivers
type DBSelector interface {
	ResourceSelector() *sqlbuilder.SelectBuilder
	UUIDResourceSelector() *sqlbuilder.SelectBuilder
	OwnedResourceSelector() *sqlbuilder.SelectBuilder
	UrlFromResourceSelector() *sqlbuilder.SelectBuilder
	UrlSelector() *sqlbuilder.SelectBuilder
}

// PartialDBSelectorImpl implements partially the DBSelector interface
// with the default selectors with non-specific DBMS functions
type PartialDBSelectorImpl struct{}

func (PartialDBSelectorImpl) UUIDResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select("uuid").From("resource")
}

func (PartialDBSelectorImpl) UrlFromResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	sb.Select("log.url", "log.json_path")
	sb.From("log_url log")
	sb.Join("resource res", "log.uuid = res.uuid")
	return sb
}

func (PartialDBSelectorImpl) UrlSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select("url", "json_path").From("log_url")
}

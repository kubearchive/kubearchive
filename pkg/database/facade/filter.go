// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import "github.com/huandu/go-sqlbuilder"

// DBFilter encapsulates all the filter functions that must be implemented by the drivers
// All its functions share the same signature
type DBFilter interface {
	KindApiVersionFilter(cond sqlbuilder.Cond, kind, apiVersion string) string
	NamespaceFilter(cond sqlbuilder.Cond, ns string) string
	NameFilter(cond sqlbuilder.Cond, name string) string
	CreationTSAndIDFilter(cond sqlbuilder.Cond, continueDate, continueId string) string
	OwnerFilter(cond sqlbuilder.Cond, ownersUuids []string) string
	UuidsFilter(cond sqlbuilder.Cond, uuids []string) string
	UuidFilter(cond sqlbuilder.Cond, uuid string) string
	ExistsLabelFilter(cond sqlbuilder.Cond, labels []string) string
	NotExistsLabelFilter(cond sqlbuilder.Cond, labels []string) string
	EqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string) string
	NotEqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string) string
	InLabelFilter(cond sqlbuilder.Cond, labels map[string][]string) string
	NotInLabelFilter(cond sqlbuilder.Cond, labels map[string][]string) string
	ContainerNameFilter(cond sqlbuilder.Cond, containerName string) string
}

// PartialDBFilterImpl implements partially the DBFilter interface
// with the default selectors with non-specific DBMS functions
type PartialDBFilterImpl struct{}

func (PartialDBFilterImpl) KindApiVersionFilter(cond sqlbuilder.Cond, kind, apiVersion string) string {
	return cond.And(cond.Equal("kind", kind), cond.Equal("api_version", apiVersion))
}

func (PartialDBFilterImpl) NamespaceFilter(cond sqlbuilder.Cond, ns string) string {
	return cond.Equal("namespace", ns)
}

func (PartialDBFilterImpl) NameFilter(cond sqlbuilder.Cond, name string) string {
	return cond.Equal("name", name)
}

func (PartialDBFilterImpl) UuidsFilter(cond sqlbuilder.Cond, uuids []string) string {
	var parsedUuids []any
	for _, v := range uuids {
		parsedUuids = append(parsedUuids, v)
	}
	return cond.In("uuid", parsedUuids...)
}

func (PartialDBFilterImpl) UuidFilter(cond sqlbuilder.Cond, uuid string) string {
	return cond.Equal("uuid", uuid)
}

func (PartialDBFilterImpl) ContainerNameFilter(cond sqlbuilder.Cond, containerName string) string {
	return cond.Equal("container_name", containerName)
}

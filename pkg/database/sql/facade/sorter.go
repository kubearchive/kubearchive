// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package facade

import "github.com/huandu/go-sqlbuilder"

// DBSorter encapsulates all the sorter functions that must be implemented by the drivers
type DBSorter interface {
	CreationTSAndIDSorter(sb *sqlbuilder.SelectBuilder) *sqlbuilder.SelectBuilder
}

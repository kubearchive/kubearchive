// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package facade

type DBCreator interface {
	GetDriverName() string
	GetConnectionString() string
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package facade

type DBCreator interface {
	GetDriverName() string
	GetConnectionString(env map[string]string) string
}

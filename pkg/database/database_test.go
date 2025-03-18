// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package database

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

var tests = []struct {
	name     string
	database Database
}{
	{
		name:     "mariadb",
		database: NewMariaDBDatabase(),
	},
	{
		name:     "postgresql",
		database: NewPostgreSQLDatabase(),
	},
}

func TestPing(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.setConn(sqlx.NewDb(db, "sqlmock"))
			mock.ExpectPing()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			assert.Nil(t, tt.database.Ping(ctx))
		})
	}
}

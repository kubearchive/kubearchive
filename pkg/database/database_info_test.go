// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestGetDatabaseEnvironmentVars(t *testing.T) {
	value := "Database"
	os.Setenv(string(DbNameEnvVar), value)
	err := fmt.Errorf(dbConnectionErrStr, DbHostEnvVar)

	tests := []struct {
		name  string
		want1 bool
		want2 error
		env   map[string]string
	}{
		{
			name:  "check values when all environment variables are set",
			want1: false, // not nil
			want2: nil,
			env: map[string]string{DbKindEnvVar: value, DbNameEnvVar: value, DbUserEnvVar: value,
				DbPasswordEnvVar: value, DbHostEnvVar: value, DbPortEnvVar: value},
		},
		{
			name:  "error when at least one environment variable is not set",
			want1: true, // nil
			want2: err,
			env: map[string]string{DbKindEnvVar: value, DbNameEnvVar: value, DbUserEnvVar: value,
				DbPasswordEnvVar: value, DbPortEnvVar: value},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range DbEnvVars {
				os.Unsetenv(v)
			}
			for k, v := range tt.env {
				os.Setenv(k, v)
			}
			got1, got2 := getDatabaseEnvironmentVars()
			if tt.want1 != (got1 == nil) {
				t.Errorf("WANT: %t\nGOT: %t", tt.want1, reflect.ValueOf(got1).Kind() == reflect.Map)
			}
			if tt.want2 != got2 && tt.want2.Error() != got2.Error() {
				t.Errorf("WANT: %s\nGOT: %s", tt.want2, got2)
			}
		})
	}
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestGetDatabaseEnvironmentVars(t *testing.T) {
	value := "Database"
	errDBHost := fmt.Errorf(dbConnectionErrStr, DbHostEnvVar)
	errDBKind := fmt.Errorf(dbConnectionErrStr, DbKindEnvVar)
	errDBName := fmt.Errorf(dbConnectionErrStr, DbNameEnvVar)

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
			want2: errDBHost,
			env: map[string]string{DbKindEnvVar: value, DbNameEnvVar: value, DbUserEnvVar: value,
				DbPasswordEnvVar: value, DbPortEnvVar: value},
		},
		{
			name:  "error when two environment variables are not set",
			want1: true, // nil
			want2: errors.Join(errDBKind, errDBName),
			env: map[string]string{DbUserEnvVar: value,
				DbPasswordEnvVar: value, DbHostEnvVar: value, DbPortEnvVar: value},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got1, got2 := NewDatabaseEnvironment()
			if tt.want1 != (got1 == nil) {
				t.Errorf("WANT: %t\nGOT: %t", tt.want1, reflect.ValueOf(got1).Kind() == reflect.Map)
			}
			if tt.want2 != got2 && tt.want2.Error() != got2.Error() {
				t.Errorf("WANT: %s\nGOT: %s", tt.want2, got2)
			}
		})
	}
}

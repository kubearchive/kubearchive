// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"os"
	"testing"
)

func TestReadDbEnvVar(t *testing.T) {
	value := "Database"
	os.Setenv(string(DbNameEnvVar), value)
	err := fmt.Errorf(dbConnectionErrStr, DbUrlEnvVar)

	tests := []struct {
		name   string
		envVar EnvVar
		want1  string
		want2  error
	}{
		{
			name:   "get value of set environment variable",
			envVar: DbNameEnvVar,
			want1:  value,
			want2:  nil,
		},
		{
			name:   "error when environment variable is not set",
			envVar: DbUrlEnvVar,
			want1:  "",
			want2:  err,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1, got2 := readDbEnvVar(tt.envVar)
			if tt.want1 != got1 {
				t.Errorf("WANT: %s\nGOT: %s", tt.want1, got1)
			}
			if tt.want2 != got2 && tt.want2.Error() != got2.Error() {
				t.Errorf("WANT: %s\nGOT: %s", tt.want2, got2)
			}
		})
	}
}

func TestConnectionStrNoEnvVars(t *testing.T) {
	_, err := ConnectionStr()
	if err == nil {
		t.Errorf("A non nil error should be returned when database environment variables are not set")
	}
}

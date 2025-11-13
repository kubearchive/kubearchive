// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"strings"
	"testing"
)

func TestExtractConnectionInfo(t *testing.T) {
	tests := []struct {
		name             string
		driver           string
		connectionString string
		expectedHost     string
		expectedPort     string
		expectedDatabase string
		expectedUser     string
		expectedSSLMode  string
	}{
		{
			name:             "PostgreSQL connection string",
			driver:           "postgres",
			connectionString: "user=testuser password=testpass dbname=testdb host=localhost port=5432 sslmode=require",
			expectedHost:     "localhost",
			expectedPort:     "5432",
			expectedDatabase: "testdb",
			expectedUser:     "testuser",
			expectedSSLMode:  "require",
		},
		{
			name:             "PostgreSQL connection string without SSL mode",
			driver:           "postgres",
			connectionString: "user=testuser password=testpass dbname=testdb host=localhost port=5432",
			expectedHost:     "localhost",
			expectedPort:     "5432",
			expectedDatabase: "testdb",
			expectedUser:     "testuser",
			expectedSSLMode:  "unknown",
		},
		{
			name:             "PostgreSQL connection string with special characters in password",
			driver:           "postgres",
			connectionString: "user=testuser password=%25123%3DHEllo%2627AZ34%21 dbname=testdb host=localhost port=5432 sslmode=disable",
			expectedHost:     "localhost",
			expectedPort:     "5432",
			expectedDatabase: "testdb",
			expectedUser:     "testuser",
			expectedSSLMode:  "disable",
		},
		{
			name:             "Non-PostgreSQL driver (should return defaults)",
			driver:           "mysql",
			connectionString: "testuser:testpass@tcp(localhost:3306)/testdb",
			expectedHost:     "unknown",
			expectedPort:     "default",
			expectedDatabase: "unknown",
			expectedUser:     "unknown",
			expectedSSLMode:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractConnectionInfo(tt.driver, tt.connectionString)

			if result.host != tt.expectedHost {
				t.Errorf("Host mismatch: got %s, want %s", result.host, tt.expectedHost)
			}
			if result.port != tt.expectedPort {
				t.Errorf("Port mismatch: got %s, want %s", result.port, tt.expectedPort)
			}
			if result.database != tt.expectedDatabase {
				t.Errorf("Database mismatch: got %s, want %s", result.database, tt.expectedDatabase)
			}
			if result.user != tt.expectedUser {
				t.Errorf("User mismatch: got %s, want %s", result.user, tt.expectedUser)
			}
			if result.sslMode != tt.expectedSSLMode {
				t.Errorf("SSL mode mismatch: got %s, want %s", result.sslMode, tt.expectedSSLMode)
			}
		})
	}
}

func TestExtractConnectionInfoEdgeCases(t *testing.T) {
	// Test with empty connection string
	result := extractConnectionInfo("postgres", "")
	if result.host != "unknown" {
		t.Errorf("Expected 'unknown' host for empty connection string, got %s", result.host)
	}

	// Test with malformed connection string
	result = extractConnectionInfo("postgres", "invalid:format:here")
	if result.host != "unknown" {
		t.Errorf("Expected 'unknown' host for malformed connection string, got %s", result.host)
	}

	// Test with very long connection string
	longString := strings.Repeat("user=test ", 1000)
	result = extractConnectionInfo("postgres", longString)
	if result.user != "test" {
		t.Errorf("Expected 'test' user for long connection string, got %s", result.user)
	}
}

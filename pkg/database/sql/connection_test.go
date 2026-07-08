// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"strings"
	"testing"
	"time"
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

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		setEnv       bool
		defaultValue int
		expected     int
	}{
		{
			name:         "env not set returns default",
			setEnv:       false,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "valid integer value",
			envValue:     "20",
			setEnv:       true,
			defaultValue: 10,
			expected:     20,
		},
		{
			name:         "invalid integer value returns default",
			envValue:     "not-a-number",
			setEnv:       true,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "empty string returns default",
			envValue:     "",
			setEnv:       true,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "zero returns default",
			envValue:     "0",
			setEnv:       true,
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "negative number returns default",
			envValue:     "-10",
			setEnv:       true,
			defaultValue: 10,
			expected:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_GET_ENV_INT"
			envMap := make(map[string]string)
			if tt.setEnv {
				envMap[key] = tt.envValue
			}
			got := getEnvInt(envMap, key, tt.defaultValue)
			if got != tt.expected {
				t.Errorf("getEnvInt() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		setEnv       bool
		defaultValue time.Duration
		expected     time.Duration
	}{
		{
			name:         "env not set returns default",
			setEnv:       false,
			defaultValue: 5 * time.Minute,
			expected:     5 * time.Minute,
		},
		{
			name:         "valid duration value",
			envValue:     "10m",
			setEnv:       true,
			defaultValue: 5 * time.Minute,
			expected:     10 * time.Minute,
		},
		{
			name:         "valid duration in seconds",
			envValue:     "30s",
			setEnv:       true,
			defaultValue: 5 * time.Minute,
			expected:     30 * time.Second,
		},
		{
			name:         "invalid duration returns default",
			envValue:     "not-a-duration",
			setEnv:       true,
			defaultValue: 5 * time.Minute,
			expected:     5 * time.Minute,
		},
		{
			name:         "empty string returns default",
			envValue:     "",
			setEnv:       true,
			defaultValue: 5 * time.Minute,
			expected:     5 * time.Minute,
		},
		{
			name:         "negative duration returns default",
			envValue:     "-5m",
			setEnv:       true,
			defaultValue: 5 * time.Minute,
			expected:     5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_GET_ENV_DURATION"
			envMap := make(map[string]string)
			if tt.setEnv {
				envMap[key] = tt.envValue
			}
			got := getEnvDuration(envMap, key, tt.defaultValue)
			if got != tt.expected {
				t.Errorf("getEnvDuration() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockMigrator records calls for testing stepMigrationsWithScripts.
type mockMigrator struct {
	version      uint
	migrateCalls []uint
	stepsCalls   int
	// versions to step through on successive Steps() calls
	stepVersions []uint
	stepIndex    int
}

func (m *mockMigrator) Version() (uint, bool, error) {
	return m.version, m.version != 0, nil
}

func (m *mockMigrator) Migrate(version uint) error {
	m.migrateCalls = append(m.migrateCalls, version)
	m.version = version
	return nil
}

func (m *mockMigrator) Steps(n int) error {
	m.stepsCalls++
	if m.stepIndex >= len(m.stepVersions) {
		return os.ErrNotExist
	}
	m.version = m.stepVersions[m.stepIndex]
	m.stepIndex++
	return nil
}

func TestStepMigrationsWithScripts_DowngradeSkipsScripts(t *testing.T) {
	mock := &mockMigrator{version: 7}

	scriptRan := false
	scripts := []scriptEntry{
		{afterVersion: 7, name: "07_08_populate.sh", path: "/fake/07_08_populate.sh"},
	}

	origExecuteScript := executeScriptFunc
	executeScriptFunc = func(path string) error {
		scriptRan = true
		return nil
	}
	defer func() { executeScriptFunc = origExecuteScript }()

	err := stepMigrationsWithScripts(mock, scripts, 5, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scriptRan {
		t.Error("scripts should NOT run during downgrade")
	}

	if len(mock.migrateCalls) != 1 || mock.migrateCalls[0] != 5 {
		t.Errorf("expected Migrate(5), got %v", mock.migrateCalls)
	}
}

func TestStepMigrationsWithScripts_UpgradeRunsScripts(t *testing.T) {
	mock := &mockMigrator{
		version:      7,
		stepVersions: []uint{8},
	}

	var scriptsRun []string
	scripts := []scriptEntry{
		{afterVersion: 7, name: "07_08_populate.sh", path: "/fake/07_08_populate.sh"},
		{afterVersion: 8, name: "08_09_index.sh", path: "/fake/08_09_index.sh"},
	}

	origExecuteScript := executeScriptFunc
	executeScriptFunc = func(path string) error {
		scriptsRun = append(scriptsRun, path)
		return nil
	}
	defer func() { executeScriptFunc = origExecuteScript }()

	err := stepMigrationsWithScripts(mock, scripts, 9, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"/fake/07_08_populate.sh", "/fake/08_09_index.sh"}
	if len(scriptsRun) != len(expected) {
		t.Fatalf("expected scripts %v, got %v", expected, scriptsRun)
	}
	for i, e := range expected {
		if scriptsRun[i] != e {
			t.Errorf("script[%d]: expected %q, got %q", i, e, scriptsRun[i])
		}
	}
}

func TestStepMigrationsWithScripts_ScriptFailureStopsMigration(t *testing.T) {
	mock := &mockMigrator{version: 7}

	scripts := []scriptEntry{
		{afterVersion: 7, name: "07_08_populate.sh", path: "/fake/07_08_populate.sh"},
	}

	origExecuteScript := executeScriptFunc
	executeScriptFunc = func(path string) error {
		return fmt.Errorf("script crashed")
	}
	defer func() { executeScriptFunc = origExecuteScript }()

	err := stepMigrationsWithScripts(mock, scripts, 9, true)
	if err == nil {
		t.Fatal("expected error from failed script")
	}

	if mock.stepsCalls != 0 {
		t.Error("should not have stepped after script failure")
	}
}

func TestDiscoverScripts(t *testing.T) {
	dir := t.TempDir()

	// Create test files: mix of migrations and scripts
	files := []string{
		"07_label_tables.up.sql",
		"07_label_tables.down.sql",
		"07_08_populate_lookups.sh",
		"08_constraints.up.sql",
		"08_09_populate_resource_labels.sh",
		"08_09_another_script.sh",
		"09_indexes.up.sql",
		"migrate_log_urls.sh", // old-style script, should be ignored
		"README.md",           // non-script, should be ignored
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("#!/bin/bash\n"), 0600); err != nil { //nolint:gosec
			t.Fatal(err)
		}
	}

	scripts, err := discoverScripts(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(scripts) != 3 {
		t.Fatalf("expected 3 scripts, got %d", len(scripts))
	}

	// Verify ordering: by afterVersion, then alphabetically
	expected := []struct {
		afterVersion uint
		name         string
	}{
		{7, "07_08_populate_lookups.sh"},
		{8, "08_09_another_script.sh"},
		{8, "08_09_populate_resource_labels.sh"},
	}

	for i, e := range expected {
		if scripts[i].afterVersion != e.afterVersion {
			t.Errorf("script[%d]: expected afterVersion %d, got %d", i, e.afterVersion, scripts[i].afterVersion)
		}
		if scripts[i].name != e.name {
			t.Errorf("script[%d]: expected name %q, got %q", i, e.name, scripts[i].name)
		}
	}
}

func TestDiscoverScriptsEmptyDir(t *testing.T) {
	dir := t.TempDir()

	scripts, err := discoverScripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 0 {
		t.Fatalf("expected 0 scripts, got %d", len(scripts))
	}
}

func TestScriptPatternMatching(t *testing.T) {
	tests := []struct {
		name    string
		matches bool
	}{
		{"07_08_populate_lookups.sh", true},
		{"1_2_x.sh", true},
		{"07_label_tables.up.sql", false},
		{"migrate_log_urls.sh", false},
		{"07_08.sh", false},  // no description
		{"07_08_.sh", false}, // no description after second underscore
	}

	for _, tt := range tests {
		got := scriptPattern.MatchString(tt.name)
		if got != tt.matches {
			t.Errorf("pattern.MatchString(%q) = %v, want %v", tt.name, got, tt.matches)
		}
	}
}

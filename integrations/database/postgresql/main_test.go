// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

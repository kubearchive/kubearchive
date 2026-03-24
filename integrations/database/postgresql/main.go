// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// scriptEntry represents a shell script to run after a specific migration version.
type scriptEntry struct {
	afterVersion uint
	name         string
	path         string
}

// scriptPattern matches filenames like "07_08_populate_lookup_tables.sh".
// The first capture group is the "after" version (the version after which this script runs).
var scriptPattern = regexp.MustCompile(`^(\d+)_\d+_.+\.sh$`)

func main() {
	start := time.Now()
	slog.Info("Starting migration process")

	migrationsDir := path.Join(os.Getenv("KO_DATA_PATH"), "migrations")
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		os.Getenv("DATABASE_USER"),
		url.QueryEscape(os.Getenv("DATABASE_PASSWORD")),
		os.Getenv("DATABASE_URL"),
		os.Getenv("DATABASE_PORT"),
		os.Getenv("DATABASE_DB"),
	)

	m, err := migrate.New(fmt.Sprintf("file://%s", migrationsDir), dbURL)
	if err != nil {
		panic(err)
	}

	scripts, err := discoverScripts(migrationsDir)
	if err != nil {
		panic(fmt.Sprintf("failed to discover scripts: %s", err))
	}

	if len(scripts) == 0 {
		// No scripts to coordinate — use the original simple path.
		err = runMigrations(m)
		if err != nil {
			panic(err)
		}
		slog.Info("Migration completed successfully", "duration", time.Since(start))
		return
	}

	var targetVersion uint
	var hasTarget bool
	if tv := os.Getenv("MIGRATION_VERSION"); tv != "" {
		v, errParse := strconv.ParseUint(tv, 10, 32)
		if errParse != nil {
			panic(fmt.Sprintf("invalid MIGRATION_VERSION %q: %s", tv, errParse))
		}
		targetVersion = uint(v)
		hasTarget = true
		slog.Info("Target migration version requested", "version", targetVersion)
	}

	// Run any scripts for the current version before stepping forward.
	// This handles the case where a previous run applied a migration but
	// its script was interrupted. Scripts are idempotent so re-running is safe.
	currentVersion, _, _ := m.Version()
	if currentVersion != 0 {
		slog.Info("Checking for scripts on current version", "version", currentVersion) //nolint:gosec // version is a uint from migrate, not user input
		if err := runScriptsForVersion(scripts, currentVersion); err != nil {
			panic(err)
		}
	}

	// Step through migrations one at a time, running scripts after each.
	for {
		if hasTarget {
			v, _, _ := m.Version()
			if v >= targetVersion {
				break
			}
		}

		stepStart := time.Now()
		err := m.Steps(1)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, migrate.ErrNoChange) {
			break
		}
		if err != nil {
			panic(err)
		}

		newVersion, _, _ := m.Version()
		slog.Info("Migration applied", "version", newVersion, "duration", time.Since(stepStart)) //nolint:gosec // version is a uint from migrate, not user input
		if err := runScriptsForVersion(scripts, newVersion); err != nil {
			panic(err)
		}
	}

	slog.Info("Migration completed successfully", "duration", time.Since(start))
}

// runMigrations applies migrations using the original simple logic (no script interleaving).
func runMigrations(m *migrate.Migrate) error {
	var err error
	if targetVersion := os.Getenv("MIGRATION_VERSION"); targetVersion != "" {
		v, errParse := strconv.ParseUint(targetVersion, 10, 32)
		if errParse != nil {
			return fmt.Errorf("invalid MIGRATION_VERSION %q: %w", targetVersion, errParse)
		}
		slog.Info("Target migration version requested", "version", v)
		err = m.Migrate(uint(v))
	} else {
		err = m.Up()
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// discoverScripts scans the migrations directory for shell scripts matching
// the naming pattern {after}_{before}_{description}.sh and returns them
// sorted by afterVersion then alphabetically by name.
func discoverScripts(migrationsDir string) ([]scriptEntry, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}

	var scripts []scriptEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := scriptPattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		afterVersion, err := strconv.ParseUint(matches[1], 10, 32)
		if err != nil {
			continue
		}
		scripts = append(scripts, scriptEntry{
			afterVersion: uint(afterVersion),
			name:         entry.Name(),
			path:         filepath.Join(migrationsDir, entry.Name()),
		})
	}

	sort.Slice(scripts, func(i, j int) bool {
		if scripts[i].afterVersion != scripts[j].afterVersion {
			return scripts[i].afterVersion < scripts[j].afterVersion
		}
		return scripts[i].name < scripts[j].name
	})

	if len(scripts) > 0 {
		slog.Info("Discovered migration scripts", "count", len(scripts))
	}

	return scripts, nil
}

// runScriptsForVersion finds and executes scripts for the given migration version.
// Scripts are idempotent, so running them multiple times is safe.
func runScriptsForVersion(scripts []scriptEntry, version uint) error {
	for _, s := range scripts {
		if s.afterVersion != version {
			continue
		}

		slog.Info("Running script", "script", s.name)
		scriptStart := time.Now()
		if err := executeScript(s.path); err != nil {
			return fmt.Errorf("script %s failed: %w", s.name, err)
		}
		slog.Info("Script completed successfully", "script", s.name, "duration", time.Since(scriptStart))
	}
	return nil
}

// executeScript runs a shell script, streaming its output to stdout/stderr.
func executeScript(scriptPath string) error {
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

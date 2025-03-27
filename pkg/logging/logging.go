// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0
package logging

import (
	"fmt"
	"log/slog"
	"os"
)

const loggingLevelEnvVar = "LOG_LEVEL"

// ConfigureLogging retrieves the variable LOG_LEVEL
// from the environment and sets the slog default logger
// with a TextHandler configured with the LOG_LEVEL level
// Returns an error if the level does not exist
func ConfigureLogging() error {
	levelText := os.Getenv(loggingLevelEnvVar)
	if levelText == "" {
		levelText = "INFO"
	}

	var level slog.Level
	err := level.UnmarshalText([]byte(levelText))
	if err != nil {
		return fmt.Errorf("Log level '%s' does not exist", levelText)
	}

	slogHandler := slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		},
	)
	slogLogger := slog.New(slogHandler)
	slog.SetDefault(slogLogger)

	return nil
}

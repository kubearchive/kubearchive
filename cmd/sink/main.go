// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log/slog"
	"os"

	"github.com/kubearchive/kubearchive/cmd/sink/logs"
	"github.com/kubearchive/kubearchive/cmd/sink/routers"
	"github.com/kubearchive/kubearchive/cmd/sink/server"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	"github.com/kubearchive/kubearchive/pkg/logging"
	kaObservability "github.com/kubearchive/kubearchive/pkg/observability"
)

var (
	version = "main"
	commit  = ""
	date    = ""
)

const (
	otelServiceName = "kubearchive.sink"
	mountPathEnvVar = "MOUNT_PATH"
)

func main() {
	if err := logging.ConfigureLogging(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	err := kaObservability.Start(otelServiceName)
	if err != nil {
		slog.Error("Could not start tracing", "err", err.Error())
		os.Exit(1)
	}

	slog.Info("Starting KubeArchive Sink", "version", version, "commit", commit, "built", date)
	slog.Info("Establishing database connection for Sink component")
	db, err := database.NewWriter()
	if err != nil {
		slog.Error("Could not connect to the database",
			"error", err.Error(),
			"component", "sink_service",
		)
		os.Exit(1)
	}
	slog.Info("Database connection established successfully",
		"component", "sink_service",
	)
	defer func(db interfaces.DBWriter) {
		slog.Info("Closing database connection", "component", "sink_service")
		err = db.CloseDB()
		if err != nil {
			slog.Error("Could not close the database connection",
				"error", err.Error(),
				"component", "sink_service",
			)
		} else {
			slog.Info("Database connection closed successfully", "component", "sink_service")
		}
	}(db)

	dynClient, err := k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		slog.Error("Could not get a kubernetes dynamic client", "error", err)
		os.Exit(1)
	}

	builder, err := logs.NewUrlBuilder()
	if err != nil {
		slog.Error("Could not enable log url creation", "error", err)
	}
	controller := routers.NewController(db, dynClient, builder)
	server := server.NewServer(controller)
	server.Serve()
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log/slog"
	"os"

	"github.com/kubearchive/kubearchive/cmd/sink/filters"
	"github.com/kubearchive/kubearchive/cmd/sink/k8s"
	"github.com/kubearchive/kubearchive/cmd/sink/logs"
	"github.com/kubearchive/kubearchive/cmd/sink/routers"
	"github.com/kubearchive/kubearchive/cmd/sink/server"
	"github.com/kubearchive/kubearchive/pkg/database"
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
	db, err := database.NewDatabase()
	if err != nil {
		slog.Error("Could not connect to the database", "err", err)
		os.Exit(1)
	}
	defer func(db database.Database) {
		err = db.CloseDB()
		if err != nil {
			slog.Error("Could not close the database connection", "error", err.Error())
		} else {
			slog.Info("Connection closed successfully")
		}
	}(db)

	clientset, err := k8s.GetKubernetesClientset()
	if err != nil {
		slog.Error("Could not get a kubernetes client", "error", err)
		os.Exit(1)
	}

	filter := filters.NewFilters(clientset)
	stopUpdating, err := filter.Update()
	if err != nil {
		slog.Error("Could not listen for updates to filters:", "error", err)
		os.Exit(1)
	}
	defer stopUpdating()
	builder, err := logs.NewUrlBuilder()
	if err != nil {
		slog.Error("Could not enable log url creation", "error", err)
	}
	dynClient, err := k8s.GetKubernetesClient()
	if err != nil {
		slog.Error("Could not get a kubernetes dynamic client", "error", err)
		os.Exit(1)
	}
	controller, err := routers.NewController(db, filter, dynClient, builder)
	if err != nil {
		slog.Error("Could not start HTTP server", "error", err)
		os.Exit(1)
	}
	server := server.NewServer(controller)
	server.Serve()
}

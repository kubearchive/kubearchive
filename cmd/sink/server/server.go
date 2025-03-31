// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cyprinus12138/otelgin"
	"github.com/gin-gonic/gin"
	"github.com/kronicler/kronicler/cmd/sink/routers"
	"github.com/kronicler/kronicler/pkg/middleware"
	"github.com/kronicler/kronicler/pkg/observability"
)

type Server struct {
	controller *routers.Controller
	router     *gin.Engine
}

func NewServer(controller *routers.Controller) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logger(middleware.LoggerConfig{PathLoggingLevel: map[string]string{
		"/livez":  "DEBUG",
		"/readyz": "DEBUG",
	}}))
	router.Use(otelgin.Middleware(""))

	router.POST("/", controller.CloudEventsHandler)

	router.GET("/livez", controller.Livez)
	router.GET("/readyz", controller.Readyz)

	observability.SetupPprof(router)

	return &Server{
		controller: controller,
		router:     router,
	}
}

func (s *Server) Serve() {
	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           s.router.Handler(),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       2 * time.Second,
	}
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			slog.Error("an error occurred while running the server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Signal received, shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Error shutting down the server", "error", err)
		os.Exit(1)
	}
}

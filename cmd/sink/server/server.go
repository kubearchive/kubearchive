// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cyprinus12138/otelgin"
	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/sink/routers"
	"github.com/kubearchive/kubearchive/pkg/middleware"
)

const enablePprofEnvVar = "KUBEARCHIVE_ENABLE_PPROF"

type Server struct {
	controller *routers.Controller
	router     *gin.Engine
}

func NewServer(controller *routers.Controller) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logger(middleware.LoggerConfig{}))
	router.Use(otelgin.Middleware(""))

	router.POST("/", controller.CloudEventsHandler)

	router.GET("/livez", controller.Livez)
	router.GET("/readyz", controller.Readyz)

	if os.Getenv(enablePprofEnvVar) == "true" {
		router.GET("/debug/pprof/", gin.WrapF(pprof.Index))
		router.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
		router.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
		router.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
		router.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
		router.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))
		router.GET("/debug/pprof/allocs", gin.WrapH(pprof.Handler("allocs")))
		router.GET("/debug/pprof/block", gin.WrapH(pprof.Handler("block")))
		router.GET("/debug/pprof/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		router.GET("/debug/pprof/heap", gin.WrapH(pprof.Handler("heap")))
		router.GET("/debug/pprof/mutex", gin.WrapH(pprof.Handler("mutex")))
		router.GET("/debug/pprof/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
	}

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

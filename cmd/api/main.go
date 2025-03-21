// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cyprinus12138/otelgin"
	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/logging"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"github.com/kubearchive/kubearchive/pkg/database"
	kaLogging "github.com/kubearchive/kubearchive/pkg/logging"
	"github.com/kubearchive/kubearchive/pkg/middleware"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	otelServiceName                   = "kubearchive.api"
	cacheExpirationAuthorizedEnvVar   = "CACHE_EXPIRATION_AUTHORIZED"
	cacheExpirationUnauthorizedEnvVar = "CACHE_EXPIRATION_UNAUTHORIZED"
)

var (
	version = "main"
	commit  = ""
	date    = ""
)

type Server struct {
	k8sClient kubernetes.Interface
	router    *gin.Engine
}

func getKubernetesClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(fmt.Sprintf("Error retrieving in-cluster k8s client config: %s", err.Error()))
	}

	config.Wrap(func(rt http.RoundTripper) http.RoundTripper { return otelhttp.NewTransport(rt) })
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("Error instantiating k8s from host %s: %s", config.Host, err.Error()))
	}
	return client
}

func NewServer(k8sClient kubernetes.Interface, controller routers.Controller, cache *cache.Cache,
	cacheExpirations *routers.CacheExpirations) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logger(middleware.LoggerConfig{PathLoggingLevel: map[string]string{
		"/livez":  "DEBUG",
		"/readyz": "DEBUG",
	}}))
	router.Use(otelgin.Middleware("")) // Empty string so the library sets the proper server

	apiGroup := router.Group("/api")
	apisGroup := router.Group("/apis")
	groups := [...]*gin.RouterGroup{apisGroup, apiGroup}
	// Set up middleware for each group
	for _, group := range groups {
		group.Use(auth.Authentication(k8sClient.AuthenticationV1().TokenReviews(), cache,
			cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		group.Use(auth.RBACAuthorization(k8sClient.AuthorizationV1().SubjectAccessReviews(), cache,
			cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		group.Use(discovery.GetAPIResource(k8sClient.Discovery().RESTClient(), cache))
		group.Use(pagination.Middleware())
	}

	router.GET("/livez", controller.Livez)
	router.GET("/readyz", controller.Readyz)

	observability.SetupPprof(router)

	creds, credsErr := logging.GetKubeArchiveLoggingCredentials()

	apisGroup.GET("/:group/:version/:resourceType", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/:name", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/:name/log",
		logging.SetLoggingCredentials(creds, credsErr), controller.GetLogURL, logging.LogRetrieval())

	apiGroup.GET("/:version/:resourceType", controller.GetResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType", controller.GetResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/:name", controller.GetResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/:name/log",
		logging.SetLoggingCredentials(creds, credsErr), controller.GetLogURL, logging.LogRetrieval())

	return &Server{
		router:    router,
		k8sClient: k8sClient,
	}
}

func main() {
	if err := kaLogging.ConfigureLogging(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	err := observability.Start(otelServiceName)
	if err != nil {
		slog.Info("Could not start opentelemetry", "error", err.Error())
	}

	slog.Info("Starting KubeArchive API", "version", version, "commit", commit, "built", date)
	cacheExpirations, err := getCacheExpirations()
	if err != nil {
		slog.Error(err.Error())
	}
	memCache := cache.New()

	db, err := database.NewReader()
	if err != nil {
		slog.Error("Could not connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer func(db database.DBReader) {
		deferErr := db.CloseDB()
		if deferErr != nil {
			slog.Error("Could not close the database connection", "error", deferErr.Error())
		} else {
			slog.Info("Database connection closed successfully")
		}
	}(db)

	controller := routers.Controller{Database: db, CacheConfiguration: *cacheExpirations}
	server := NewServer(getKubernetesClient(), controller, memCache, cacheExpirations)
	httpServer := http.Server{
		Addr:    "0.0.0.0:8081",
		Handler: server.router.Handler(),
		// We do not accept bodies yet, because we are read-only, so we set a
		// small timeout for headers and complete request. This prevents the
		// SlowLoris attack (see Wikipedia) by closing open connections fast
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       2 * time.Second,
	}

	go func() {
		shutdownErr := httpServer.ListenAndServeTLS("/etc/kubearchive/ssl/tls.crt", "/etc/kubearchive/ssl/tls.key")
		if shutdownErr != nil && shutdownErr != http.ErrServerClosed {
			slog.Error("Error listening", "error", shutdownErr.Error())
			os.Exit(1)
		}
	}()

	// This blocks until `quitChan` gets a signal injected by `signal.Notify`.
	// After the injection the execution continues and the shutdown happens.
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	<-quitChan

	slog.Info("Shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = httpServer.Shutdown(ctx)
	if err != nil {
		slog.Error("Error shutting down the server", "error", err.Error())
		os.Exit(1)
	}

	// This blocks until the context expires
	<-ctx.Done()
	slog.Debug("Shutdown reached timeout")
}

func getCacheExpirations() (*routers.CacheExpirations, error) {
	expirationAuthorizedString := os.Getenv(cacheExpirationAuthorizedEnvVar)
	if expirationAuthorizedString == "" {
		return nil, fmt.Errorf("The environment variable '%s' should be set.", cacheExpirationAuthorizedEnvVar)
	}

	expirationAuthorized, err := time.ParseDuration(expirationAuthorizedString)
	if err != nil {
		return nil, fmt.Errorf("'%s': '%s' could not be parsed into a duration: %s", cacheExpirationAuthorizedEnvVar, expirationAuthorizedString, err)
	}

	expirationUnauthorizedString := os.Getenv(cacheExpirationUnauthorizedEnvVar)
	if expirationUnauthorizedString == "" {
		return nil, fmt.Errorf("The environment variable '%s' should be set.", cacheExpirationUnauthorizedEnvVar)
	}

	expirationUnauthorized, err := time.ParseDuration(expirationUnauthorizedString)
	if err != nil {
		return nil, fmt.Errorf("'%s': '%s' could not be parsed into a duration: %s", cacheExpirationUnauthorizedEnvVar, expirationUnauthorizedString, err)
	}

	return &routers.CacheExpirations{
		Authorized:   expirationAuthorized,
		Unauthorized: expirationUnauthorized,
	}, nil
}

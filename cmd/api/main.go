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
	"strconv"
	"syscall"
	"time"

	"github.com/Cyprinus12138/otelgin"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/logging"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	kaLogging "github.com/kubearchive/kubearchive/pkg/logging"
	"github.com/kubearchive/kubearchive/pkg/middleware"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/client-go/kubernetes"
)

const (
	otelServiceName                   = "kubearchive.api"
	cacheExpirationAuthorizedEnvVar   = "CACHE_EXPIRATION_AUTHORIZED"
	cacheExpirationUnauthorizedEnvVar = "CACHE_EXPIRATION_UNAUTHORIZED"

	rateLimitOverallRPSEnvVar      = "API_RATE_LIMIT_OVERALL_RPS"
	rateLimitLogRPSEnvVar          = "API_RATE_LIMIT_LOG_RPS"
	rateLimitOverallBurstEnvVar    = "API_RATE_LIMIT_OVERALL_BURST"
	rateLimitLogBurstEnvVar        = "API_RATE_LIMIT_LOG_BURST"
	maxConcurrentRequestsEnvVar    = "API_MAX_CONCURRENT_REQUESTS"
	maxConcurrentLogRequestsEnvVar = "API_MAX_CONCURRENT_LOG_REQUESTS"
)

var (
	version = "main"
	commit  = ""
	date    = ""
)

// RateLimitConfig holds the configuration for the API rate limiters.
type RateLimitConfig struct {
	OverallRPS         float64
	LogRPS             float64
	OverallBurst       int
	LogBurst           int
	MaxConcurrent      int
	MaxConcurrentLog   int
}

type Server struct {
	k8sClient kubernetes.Interface
	router    *gin.Engine
}

func NewServer(k8sClient kubernetes.Interface, controller routers.Controller, cache *cache.Cache,
	cacheExpirations *routers.CacheExpirations, rateLimits RateLimitConfig) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(otelgin.Middleware("", otelgin.WithDisableGinErrorsOnMetrics(true))) // Empty string so the library sets the proper server
	router.Use(kaLogging.TracingMiddleware())
	router.Use(middleware.Logger(middleware.LoggerConfig{PathLoggingLevel: map[string]string{
		"/livez":  "DEBUG",
		"/readyz": "DEBUG",
	}}))

	apiGroup := router.Group("/api")
	apisGroup := router.Group("/apis")
	groups := [...]*gin.RouterGroup{apisGroup, apiGroup}
	// Set up middleware for each group
	for _, group := range groups {
		group.Use(gzip.Gzip(gzip.DefaultCompression))
		group.Use(auth.Authentication(k8sClient.AuthenticationV1().TokenReviews(), cache,
			cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		group.Use(auth.Impersonation(k8sClient.AuthorizationV1().SubjectAccessReviews(), cache,
			cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		group.Use(discovery.GetAPIResource(k8sClient.Discovery().RESTClient(), cache))
		group.Use(auth.RBACAuthorization(k8sClient.AuthorizationV1().SubjectAccessReviews(), cache,
			cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		group.Use(pagination.Middleware())
		group.Use(middleware.RateLimiter(rateLimits.OverallRPS, rateLimits.OverallBurst))
		group.Use(middleware.ConcurrentLimiter(rateLimits.MaxConcurrent))
	}

	router.GET("/livez", controller.Livez)
	router.GET("/readyz", controller.Readyz)

	logMiddleware := []gin.HandlerFunc{
		middleware.RateLimiter(rateLimits.LogRPS, rateLimits.LogBurst),
		middleware.ConcurrentLimiter(rateLimits.MaxConcurrentLog),
	}

	apisGroup.GET("/:group/:version/:resourceType", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/:name", controller.GetResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/:name/log",
		append(logMiddleware, logging.SetLoggingConfig(), controller.GetLogURL, logging.LogRetrieval())...)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/uid/:uid", controller.GetResourceByUID)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/uid/:uid/log",
		append(logMiddleware, logging.SetLoggingConfig(), controller.GetLogURL, logging.LogRetrieval())...)

	apiGroup.GET("/:version/:resourceType", controller.GetResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType", controller.GetResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/:name", controller.GetResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/:name/log",
		append(logMiddleware, logging.SetLoggingConfig(), controller.GetLogURL, logging.LogRetrieval())...)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/uid/:uid", controller.GetResourceByUID)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/uid/:uid/log",
		append(logMiddleware, logging.SetLoggingConfig(), controller.GetLogURL, logging.LogRetrieval())...)

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
	rateLimits := getRateLimitConfig()
	memCache := cache.New()

	slog.Info("Establishing database connection for API server")
	db, err := database.NewReader()
	if err != nil {
		slog.Error("Could not connect to database",
			"error", err.Error(),
			"component", "api_server",
		)
		os.Exit(1)
	}
	slog.Info("Database connection established successfully",
		"component", "api_server",
	)
	defer func(db interfaces.DBReader) {
		slog.Info("Closing database connection", "component", "api_server")
		deferErr := db.CloseDB()
		if deferErr != nil {
			slog.Error("Could not close the database connection",
				"error", deferErr.Error(),
				"component", "api_server",
			)
		} else {
			slog.Info("Database connection closed successfully", "component", "api_server")
		}
	}(db)

	controller := routers.Controller{Database: db, CacheConfiguration: *cacheExpirations}
	k8sClient, err := k8sclient.NewInstrumentedKubernetesClient()
	if err != nil {
		slog.Error("Could not create instrumented kubernetes client", "error", err.Error())
		os.Exit(1)
	}

	server := NewServer(k8sClient, controller, memCache, cacheExpirations, rateLimits)
	httpServer := http.Server{
		Addr:    "0.0.0.0:8081",
		Handler: server.router.Handler(),
		// ReadHeaderTimeout prevents SlowLoris attacks by closing connections
		// that take too long to send headers. We intentionally do NOT set
		// ReadTimeout because it cancels the request context, which would
		// kill long-running streaming responses (e.g. paginated log retrieval).
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		shutdownErr := httpServer.ListenAndServeTLS("/etc/kubearchive/ssl/tls.crt", "/etc/kubearchive/ssl/tls.key")
		if shutdownErr != nil && shutdownErr != http.ErrServerClosed {
			slog.Error("Error listening", "error", shutdownErr.Error())
			os.Exit(1)
		}
	}()

	if os.Getenv(observability.EnablePprofEnvVar) == "true" {
		pprofServer := observability.GetObservabilityServer()
		go func() {
			shutdownErr := pprofServer.ListenAndServeTLS("/etc/kubearchive/ssl/tls.crt", "/etc/kubearchive/ssl/tls.key")
			if shutdownErr != nil && shutdownErr != http.ErrServerClosed {
				slog.Error("Error listening pprof server", "error", shutdownErr.Error())
				os.Exit(1)
			}
		}()
	}

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

func getRateLimitConfig() RateLimitConfig {
	cfg := RateLimitConfig{
		OverallRPS:       20,
		LogRPS:           2,
		OverallBurst:     20,
		LogBurst:         2,
		MaxConcurrent:    50,
		MaxConcurrentLog: 5,
	}
	if v := os.Getenv(rateLimitOverallRPSEnvVar); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.OverallRPS = parsed
		} else {
			slog.Warn("Could not parse env var, using default", "var", rateLimitOverallRPSEnvVar, "value", v)
		}
	}
	if v := os.Getenv(rateLimitLogRPSEnvVar); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LogRPS = parsed
		} else {
			slog.Warn("Could not parse env var, using default", "var", rateLimitLogRPSEnvVar, "value", v)
		}
	}
	if v := os.Getenv(rateLimitOverallBurstEnvVar); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.OverallBurst = parsed
		} else {
			slog.Warn("Could not parse env var, using default", "var", rateLimitOverallBurstEnvVar, "value", v)
		}
	}
	if v := os.Getenv(rateLimitLogBurstEnvVar); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.LogBurst = parsed
		} else {
			slog.Warn("Could not parse env var, using default", "var", rateLimitLogBurstEnvVar, "value", v)
		}
	}
	if v := os.Getenv(maxConcurrentRequestsEnvVar); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.MaxConcurrent = parsed
		} else {
			slog.Warn("Could not parse env var, using default", "var", maxConcurrentRequestsEnvVar, "value", v)
		}
	}
	if v := os.Getenv(maxConcurrentLogRequestsEnvVar); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.MaxConcurrentLog = parsed
		} else {
			slog.Warn("Could not parse env var, using default", "var", maxConcurrentLogRequestsEnvVar, "value", v)
		}
	}
	return cfg
}

func getCacheExpirations() (*routers.CacheExpirations, error) {
	expirationAuthorizedString := os.Getenv(cacheExpirationAuthorizedEnvVar)
	if expirationAuthorizedString == "" {
		return nil, fmt.Errorf("the environment variable '%s' should be set", cacheExpirationAuthorizedEnvVar)
	}

	expirationAuthorized, err := time.ParseDuration(expirationAuthorizedString)
	if err != nil {
		return nil, fmt.Errorf("'%s': '%s' could not be parsed into a duration: %s", cacheExpirationAuthorizedEnvVar, expirationAuthorizedString, err)
	}

	expirationUnauthorizedString := os.Getenv(cacheExpirationUnauthorizedEnvVar)
	if expirationUnauthorizedString == "" {
		return nil, fmt.Errorf("the environment variable '%s' should be set", cacheExpirationUnauthorizedEnvVar)
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

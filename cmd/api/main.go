// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/middleware"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
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

func NewServer(k8sClient kubernetes.Interface, controller routers.Controller, cache *cache.Cache, cacheExpirations *routers.CacheExpirations) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logger)
	router.Use(otelgin.Middleware("")) // Empty string so the library sets the proper server

	apiGroup := router.Group("/api")
	apisGroup := router.Group("/apis")
	groups := [...]*gin.RouterGroup{apisGroup, apiGroup}
	// Set up middleware for each group
	for _, group := range groups {
		group.Use(auth.Authentication(k8sClient.AuthenticationV1().TokenReviews(), cache, cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		group.Use(auth.RBACAuthorization(k8sClient.AuthorizationV1().SubjectAccessReviews(), cache, cacheExpirations.Authorized, cacheExpirations.Unauthorized))
		// TODO - Probably want to use cache for the discovery client
		// See https://pkg.go.dev/k8s.io/client-go/discovery/cached/disk#NewCachedDiscoveryClientForConfig
		group.Use(discovery.GetAPIResource(k8sClient.Discovery().RESTClient(), cache))
	}

	router.GET("/livez", controller.Livez)
	router.GET("/readyz", controller.Readyz)

	apisGroup.GET("/:group/:version/:resourceType", controller.GetAllResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType", controller.GetNamespacedResources)
	apisGroup.GET("/:group/:version/namespaces/:namespace/:resourceType/:name", controller.GetNamespacedResourceByName)

	apiGroup.GET("/:version/:resourceType", controller.GetAllCoreResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType", controller.GetNamespacedCoreResources)
	apiGroup.GET("/:version/namespaces/:namespace/:resourceType/:name", controller.GetNamespacedCoreResourceByName)

	return &Server{
		router:    router,
		k8sClient: k8sClient,
	}
}

func main() {
	slog.Info("Starting KubeArchive API", "version", version, "commit", commit, "built", date)
	err := observability.Start(otelServiceName)
	if err != nil {
		slog.Info("Could not start opentelemetry", "error", err.Error())
	}

	cacheExpirations, err := getCacheExpirations()
	if err != nil {
		slog.Error(err.Error())
	}
	memCache := cache.New()

	db, err := database.NewDatabase()
	if err != nil {
		slog.Error("Could not connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer func(db database.DBInterface) {
		err := db.CloseDB()
		if err != nil {
			slog.Error("Could not close the database connection", "error", err.Error())
		} else {
			slog.Info("Connection closed successfully")
		}
	}(db)

	controller := routers.Controller{Database: db, CacheConfiguration: *cacheExpirations}

	server := NewServer(getKubernetesClient(), controller, memCache, cacheExpirations)
	err = server.router.RunTLS("0.0.0.0:8081", "/etc/kubearchive/ssl/tls.crt", "/etc/kubearchive/ssl/tls.key")
	if err != nil {
		slog.Error("Could not run server on localhost", "error", err.Error())
		os.Exit(1)
	}
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

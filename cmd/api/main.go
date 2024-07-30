// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const otelServiceName = "kubearchive.api"

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

func NewServer(k8sClient kubernetes.Interface, controller routers.Controller) *Server {
	router := gin.Default()
	router.Use(otelgin.Middleware("")) // Empty string so the library sets the proper server
	router.Use(auth.Authentication(k8sClient.AuthenticationV1().TokenReviews()))
	router.Use(auth.RBACAuthorization(k8sClient.AuthorizationV1().SubjectAccessReviews()))
	// TODO - Probably want to use cache for the discovery client
	// See https://pkg.go.dev/k8s.io/client-go/discovery/cached/disk#NewCachedDiscoveryClientForConfig
	router.Use(discovery.GetAPIResource(k8sClient.Discovery().RESTClient()))
	router.GET("/apis/:group/:version/:resourceType", controller.GetAllResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType", controller.GetNamespacedResources)

	return &Server{
		router:    router,
		k8sClient: k8sClient,
	}
}

func main() {
	err := observability.Start(otelServiceName)
	if err != nil {
		log.Printf("Could not start opentelemetry: %s", err)
	}
	db, err := database.NewDatabase()
	if err != nil {
		log.Fatalf("Could not connect to database: %s", err)
	}
	controller := routers.Controller{Database: db}
	server := NewServer(getKubernetesClient(), controller)
	err = server.router.RunTLS("localhost:8081", "/etc/kubearchive/ssl/tls.crt", "/etc/kubearchive/ssl/tls.key")
	if err != nil {
		log.Printf("Could not run server on localhost: %s", err)
	}
}

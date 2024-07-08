// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"github.com/kubearchive/kubearchive/pkg/database"
	"log"

	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
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
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("Error instantiating k8s from host %s: %s", config.Host, err.Error()))
	}
	return client
}

func NewServer(k8sClient kubernetes.Interface, controller routers.Controller) *Server {
	router := gin.Default()
	router.Use(otelgin.Middleware("kubearchive.api"))
	router.Use(auth.Authentication(k8sClient.AuthenticationV1().TokenReviews()))
	router.Use(auth.RBACAuthorization(k8sClient.AuthorizationV1().SubjectAccessReviews()))
	// TODO Add middleware for the db connection
	router.GET("/apis/:group/:version/:resourceType", controller.GetAllResources)

	return &Server{
		router:    router,
		k8sClient: k8sClient,
	}
}

func main() {
	err := observability.Start()
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

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"github.com/kubearchive/kubearchive/cmd/api/auth"
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

var k8sClient = func() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(fmt.Sprintf("Error retrieving in-cluster k8s client config: %s", err.Error()))
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("Error instantiating k8s from host %s: %s", config.Host, err.Error()))
	}
	return client
}()

func setupRouter() *gin.Engine {
	router := gin.Default()
	router.Use(otelgin.Middleware("kubearchive.api"))
	// TODO Add AuthN middleware
	router.Use(auth.RBACAuthorization(k8sClient.AuthorizationV1().SubjectAccessReviews()))
	router.GET("/apis/:group/:version/:resourceType", routers.GetAllResources)
	return router
}

func main() {
	err := observability.Start()
	if err != nil {
		log.Printf("Could not start opentelemetry: %s", err)
	}

	router := setupRouter()
	router.Run("localhost:8081")
}

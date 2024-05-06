// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"log"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func setupRouter() *gin.Engine {
	router := gin.Default()
	router.Use(otelgin.Middleware("kubearchive.api"))
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

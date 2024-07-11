// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/database/fake"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
)

var testResources = []models.Resource{
	{Kind: "Crontab", ApiVersion: "stable.example.com/v1", Status: nil, Spec: nil, Metadata: nil},
}

func setupRouter() *gin.Engine {
	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources)}
	router.GET("/apis/:group/:version/:resourceType", ctrl.GetAllResources)
	return router
}

func TestGetAllResources(t *testing.T) {
	router := setupRouter()

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources []models.Resource
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, resources, testResources)
}

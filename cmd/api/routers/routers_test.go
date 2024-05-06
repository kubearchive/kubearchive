// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupRouter() *gin.Engine {
	router := gin.Default()
	router.GET("/apis/:group/:version/:resourceType", GetAllResources)
	return router
}

func TestGetAllResources(t *testing.T) {
	router := setupRouter()

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/apis/stable.example.com/v1/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	resources := resources{}
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, resources.Kind, "crontab")
	assert.Equal(t, resources.APIVersion, "stable.example.com/v1")
	assert.Greater(t, len(resources.Items), 0)
}

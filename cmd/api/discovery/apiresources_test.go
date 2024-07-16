package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeK8s "k8s.io/client-go/kubernetes/fake"
)

var testAPIResource = metav1.APIResource{
	Kind:         "Crontab",
	Name:         "crontabs",
	Group:        "stable.example.com",
	Version:      "v1",
	SingularName: "crontab",
	Namespaced:   true}

var testAPIResourceList = []*metav1.APIResourceList{
	{
		GroupVersion: "stable.example.com/v1",
		APIResources: []metav1.APIResource{testAPIResource},
	}}

func TestGetAPIResource(t *testing.T) {

	validGroup := testAPIResourceList[0].APIResources[0].Group
	validVersion := testAPIResourceList[0].APIResources[0].Version
	validResource := testAPIResourceList[0].APIResources[0].Name

	k8sClient := fakeK8s.NewSimpleClientset()
	k8sClient.Resources = testAPIResourceList
	fakeDiscovery := k8sClient.Discovery()

	tests := []struct {
		name         string
		group        string
		version      string
		resource     string
		expectedCode int
	}{
		{
			name:         "Invalid group",
			group:        "invalid",
			version:      validVersion,
			resource:     validResource,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Invalid version",
			group:        validGroup,
			version:      "v2",
			resource:     validResource,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Invalid resource name",
			group:        validGroup,
			version:      validVersion,
			resource:     "invalid",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Valid resource",
			group:        validGroup,
			version:      validVersion,
			resource:     validResource,
			expectedCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
			c.AddParam("group", tc.group)
			c.AddParam("version", tc.version)
			c.AddParam("resourceType", tc.resource)
			GetAPIResource(fakeDiscovery)(c)
			apiResource, _ := c.Get("apiResource")
			assert.Equal(t, tc.expectedCode, res.Code)
			if tc.expectedCode == http.StatusOK {
				assert.Equal(t, apiResource, testAPIResource)
			} else {
				assert.Nil(t, apiResource)
			}
		})
	}
}

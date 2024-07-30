package discovery

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakeK8s "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	fakeRest "k8s.io/client-go/rest/fake"
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIResourceList",
			APIVersion: "v1",
		},
		GroupVersion: "stable.example.com/v1",
		APIResources: []metav1.APIResource{testAPIResource},
	}}

func TestGetAPIResource(t *testing.T) {

	validGroup := testAPIResourceList[0].APIResources[0].Group
	validVersion := testAPIResourceList[0].APIResources[0].Version
	validResource := testAPIResourceList[0].APIResources[0].Name

	k8sClient := fakeK8s.NewSimpleClientset()
	k8sClient.Resources = testAPIResourceList

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

			apiResources, err := k8sClient.Discovery().ServerResourcesForGroupVersion(fmt.Sprintf("%s/%s", tc.group, tc.version))
			status := http.StatusOK
			response := ""
			if err != nil {
				response = err.Error()
				status = http.StatusBadRequest
			} else {
				bodyBytes, err := json.Marshal(apiResources)
				if err != nil {
					t.Fatalf("Error while serializing apiResources: %s", err)
				}
				response = string(bodyBytes)

			}
			// mock the restClient with the given status and response
			restClient := &fakeRest.RESTClient{
				Client: fakeRest.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
					resp := &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(strings.NewReader(response)),
					}
					return resp, nil
				}),
				NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
				GroupVersion:         schema.GroupVersion{Version: "v1"},
				VersionedAPIPath:     fmt.Sprintf("/apis/%s/%s", tc.group, tc.version),
			}

			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
			c.AddParam("group", tc.group)
			c.AddParam("version", tc.version)
			c.AddParam("resourceType", tc.resource)
			GetAPIResource(restClient)(c)
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

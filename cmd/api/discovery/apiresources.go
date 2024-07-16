package discovery

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/discovery"

	"github.com/kubearchive/kubearchive/cmd/api/abort"
)

// GetAPIResource set apiResource attribute in the context based on the path parameters group version and resourceType
func GetAPIResource(discoveryInterface discovery.DiscoveryInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupVersion := fmt.Sprintf("%s/%s", c.Param("group"), c.Param("version"))
		resourceList, err := discoveryInterface.ServerResourcesForGroupVersion(groupVersion)
		if err != nil {
			abort.Abort(c, err.Error(), http.StatusBadRequest)
			return
		}
		resourceName := c.Param("resourceType")
		for _, resource := range resourceList.APIResources {
			if resource.Name == resourceName {
				c.Set("apiResource", resource)
				c.Next()
				return
			}
		}
		abort.Abort(c,
			fmt.Sprintf("Unable to find the API resource %s in the Kubernetes cluster", resourceName),
			http.StatusBadRequest)
	}
}

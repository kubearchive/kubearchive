package discovery

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/kubearchive/kubearchive/cmd/api/abort"
)

// GetAPIResource set apiResource attribute in the context based on the path parameters group version and resourceType
//
// This function relied on the DiscoveryClient's ServerResourcesForGroupVersion function. However it does not accept a context
// so we had to implement the same call to be able to pass the context so telemetry traces are tied together
// @rh-hemartin opened a ticket to allow for a context on that function, see https://github.com/kubernetes/client-go/issues/1370
func GetAPIResource(client rest.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupVersion := fmt.Sprintf("%s/%s", c.Param("group"), c.Param("version"))

		// TODO: when we support the core API we need to change this
		url := fmt.Sprintf("/apis/%s", groupVersion)
		result := client.Get().AbsPath(url).Do(c.Request.Context())

		if result.Error() != nil {
			status := 0
			result.StatusCode(&status)
			abort.Abort(c, fmt.Errorf("Unable to retrieve information from '%s', error: %w", url, result.Error()).Error(), status)
			return
		}

		resources := &metav1.APIResourceList{
			GroupVersion: groupVersion,
		}

		err := result.Into(resources)
		if err != nil {
			abort.Abort(c, fmt.Errorf("Unable to deserialize result from '%s', error: %w", url, err).Error(), http.StatusInternalServerError)
			return
		}

		resourceName := c.Param("resourceType")
		for _, resource := range resources.APIResources {
			if resource.Name == resourceName {
				c.Set("apiResource", resource)
				return
			}
		}
		abort.Abort(c,
			fmt.Sprintf("Unable to find the API resource %s in the Kubernetes cluster", resourceName),
			http.StatusBadRequest)
	}
}

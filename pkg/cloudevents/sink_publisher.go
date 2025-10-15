// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cloudevents

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	otelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	ce "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

type SinkCloudEventPublisher struct {
	httpClient client.Client
	target     string
	source     string
}

func NewSinkCloudEventPublisher(source string) (*SinkCloudEventPublisher, error) {
	scep := &SinkCloudEventPublisher{
		source: source,
	}

	// Overrides defaultRetriableErrors in
	// https://github.com/cloudevents/sdk-go/blob/main/v2/protocol/http/protocol.go
	var retriableStatuses = map[int]bool{
		http.StatusNotFound:              true, // 404
		http.StatusRequestEntityTooLarge: true, // 413
		http.StatusTooEarly:              true, // 425
		http.StatusTooManyRequests:       true, // 429
		http.StatusInternalServerError:   true, // 500
		http.StatusBadGateway:            true, // 502
		http.StatusServiceUnavailable:    true, // 503
		http.StatusGatewayTimeout:        true, // 504
	}
	var ceOption = []cehttp.Option{
		cehttp.WithIsRetriableFunc(func(statusCode int) bool {
			_, retriable := retriableStatuses[statusCode]
			return retriable
		},
		)}

	var err error
	if scep.httpClient, err = otelObs.NewClientHTTP(ceOption, []client.Option{}); err != nil {
		slog.Error("Failed to create client", "error", err)
		return nil, err
	}

	scep.target = getSinkServiceUrl()

	return scep, nil
}

func (scep *SinkCloudEventPublisher) Send(ctx context.Context, eventType string, resource map[string]interface{}) ce.Result {
	event := ce.NewEvent()
	event.SetSource(scep.source)
	event.SetType(eventType)
	if err := event.SetData(ce.ApplicationJSON, resource); err != nil {
		slog.Error("Error setting cloudevent data", "error", err)
		return ce.NewResult(err.Error())
	}

	event.SetExtension("apiversion", resource["apiVersion"].(string))
	event.SetExtension("kind", resource["kind"].(string))
	metadata := resource["metadata"].(map[string]interface{})
	event.SetExtension("name", metadata["name"])
	event.SetExtension("namespace", metadata["namespace"])

	ectx := ce.ContextWithTarget(ctx, scep.target)

	return scep.httpClient.Send(ectx, event)
}

// getSinkServiceUrl constructs the URL for the local sink service
func getSinkServiceUrl() string {
	// Construct the service URL: http://<service-name>.<namespace>.svc.cluster.local:<port>
	// The sink service runs on port 80 by default
	serviceUrl := fmt.Sprintf("http://%s.%s.svc.cluster.local:80", constants.KubeArchiveSinkName, constants.KubeArchiveNamespace)
	slog.Info("Using sink service URL", "url", serviceUrl)
	return serviceUrl
}

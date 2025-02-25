// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"
)

func NewCEResult(statusCode int, err error) protocol.Result {
	if statusCode >= 200 && statusCode < 400 {
		// Cloud Event Processed successfully
		return cloudevents.NewHTTPResult(statusCode, "cloud event processed successfully")
	} else if err == nil {
		return cloudevents.NewHTTPResult(statusCode, "an error occurred while processing the cloud event")
	}
	return cloudevents.NewHTTPResult(statusCode, "an error occurred while processing the cloud event", err.Error())
}

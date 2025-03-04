// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"
)

// knative eventing expects that a response to a cloud event has an empty body, so NewHTTPResult should be called with
// an empty string.
func NewCEResult(statusCode int) protocol.Result {
	return cloudevents.NewHTTPResult(statusCode, "")
}

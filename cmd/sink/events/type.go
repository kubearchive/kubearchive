// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const DeleteTypeEventSuffix = ".delete"

func IsDeleteType(event cloudevents.Event) bool {
	return strings.HasSuffix(event.Type(), DeleteTypeEventSuffix)
}

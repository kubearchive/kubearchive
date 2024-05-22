// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

var logger = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)

func receive(event cloudevents.Event) {
	logger.Println("received CloudEvent: ", event.ID())
	logger.Printf("%s\n", event.String())
}

func main() {
	client, err := cloudevents.NewClientHTTP()
	if err != nil {
		logger.Fatalf("failed to create CloudEvents HTTP client: %s", err.Error())
	}

	err = client.StartReceiver(context.Background(), receive)
	if err != nil {
		logger.Fatalf("failed to start receiving CloudEvents: %s", err.Error())
	}
}

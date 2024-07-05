// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"reflect"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func TestNewArchiveEntryValid(t *testing.T) {
	event := cloudevents.NewEvent()
	err := event.Context.SetExtension(apiVersionExtension, "v1")
	if err != nil {
		t.Errorf("failed to set extension on test event")
	}
	err = event.Context.SetExtension(kindExtension, "Event")
	if err != nil {
		t.Errorf("failed to set extension on test event")
	}
	err = event.Context.SetExtension(nameExtension, "foo")
	if err != nil {
		t.Errorf("failed to set extension on test event")
	}
	err = event.Context.SetExtension(namespaceExtension, "bar")
	if err != nil {
		t.Errorf("failed to set extension on test event")
	}
	metadata := map[string]string{
		"resourceVersion": "new-version",
	}
	data := map[string]interface{}{
		"firstTimestamp": "0000",
		"lastTimestamp":  "0001",
		"metadata":       metadata,
	}
	err = event.SetData(cloudevents.ApplicationJSON, data)
	if err != nil {
		t.Errorf("failed to set payload data for test event")
	}
	entry := &ArchiveEntry{
		Event: event,
		Data: ResourceData{
			Created:     "0000",
			LastUpdated: "0001",
			Metadata: ResourceMetadata{
				ResourceVersion: "new-version",
			},
		},
		ApiVersion: "v1",
		Kind:       "Event",
		Name:       "foo",
		Namespace:  "bar",
	}

	tests := []struct {
		name  string
		event cloudevents.Event
		want1 *ArchiveEntry
		want2 error
	}{
		{
			name:  "New ArchiveEntry created from valid cloudevent",
			event: event,
			want1: entry,
			want2: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1, got2 := NewArchiveEntry(tt.event)
			if !reflect.DeepEqual(tt.want1, got1) {
				t.Errorf("WANT: %+v\nGOT: %+v", tt.want1, got1)
			}
			if got2 != nil {
				t.Errorf("WANT: %+v\nGOT: %+v", tt.want2, got2)
			}
		})
	}
}

func TestNewArchiveEntryInvalid(t *testing.T) {
	event := cloudevents.NewEvent()

	tests := []struct {
		name  string
		event cloudevents.Event
		want  *ArchiveEntry
	}{
		{
			name:  "Error returned when creating ArchiveEntry from cloudevent with unexpected schema",
			event: event,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1, got2 := NewArchiveEntry(tt.event)
			if got1 != nil {
				t.Errorf("WANT: %+v\nGOT: %+v", nil, got1)
			}
			if got2 == nil {
				t.Errorf("NewArchiveEntry() should not return a nil error given a cloudevent with an unexpected scehma")
			}
		})
	}
}

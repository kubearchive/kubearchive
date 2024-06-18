// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"database/sql"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
)

const (
	apiVersionExtension = "apiversion"
	kindExtension       = "kind"
	nameExtension       = "name"
	namespaceExtension  = "namespace"

	writeQuery = "INSERT INTO ArchiveMeta (api_version, kind, name, namespace, resource_version, data, created_ts, updated_ts) Values ($1, $2, $3, $4, $5, $6, $7, $8)"
)

// Represents the fields in a resource's metadata that are need to write a resource into the database
type ResourceMetadata struct {
	ResourceVersion string `json:"resourceVersion"`
}

// Respresents the fields in the data field of a cloudevent to write a resource into the database
type ResourceData struct {
	Created     string           `json:"firstTimestamp"`
	LastUpdated string           `json:"lastTimestamp"`
	Metadata    ResourceMetadata `json:"metadata"`
}

// A container for all of the data required for writing a Kubernetes resource into the database
type ArchiveEntry struct {
	Event      cloudevents.Event
	Data       ResourceData
	ApiVersion string
	Kind       string
	Name       string
	Namespace  string
}

// implement the Stringers interface so that golang can convert ArchiveEntries to string
func (entry *ArchiveEntry) String() string {
	return string(entry.Event.Data())
}

// the timestamp when the resource was created
func (entry *ArchiveEntry) Created() string {
	return entry.Data.Created
}

// the timestamp when the resource was last updated
func (entry *ArchiveEntry) LastUpdated() string {
	return entry.Data.LastUpdated
}

// the version of the resources
func (entry *ArchiveEntry) ResourceVersion() string {
	return entry.Data.Metadata.ResourceVersion
}

// checks that the cloudevents has the appropriate extensions set and have values that are the right type. Additionally
// it checks that the cloudevent's data has the necessary fields. If all conditions are not met, it returns an error
func NewArchiveEntry(event cloudevents.Event) (*ArchiveEntry, error) {
	eventExtensions := event.Extensions()
	apiVersion, err := types.ToString(eventExtensions[apiVersionExtension])
	if err != nil {
		return nil, err
	}
	kind, err := types.ToString(eventExtensions[kindExtension])
	if err != nil {
		return nil, err
	}
	name, err := types.ToString(eventExtensions[nameExtension])
	if err != nil {
		return nil, err
	}
	namespace, err := types.ToString(eventExtensions[namespaceExtension])
	if err != nil {
		return nil, err
	}
	payload := ResourceData{}
	err = event.DataAs(&payload)
	if err != nil {
		return nil, err
	}

	return &ArchiveEntry{
			Event:      event,
			Data:       payload,
			ApiVersion: apiVersion,
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		},
		nil
}

// Writes the kubernetes resource held by entry to the database using the provided connection string. Timeouts and
// deadlines for writing to the database are handled by ctx. If the database transaction fails, it will return an
// error and attempt to rollback the transaction
func (entry ArchiveEntry) WriteToDatabase(ctx context.Context, dbConn *sql.DB) error {
	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %s", err)
	}
	_, execErr := tx.ExecContext(ctx,
		writeQuery,
		entry.ApiVersion,
		entry.Kind,
		entry.Name,
		entry.Namespace,
		entry.ResourceVersion(),
		entry.String(),
		entry.Created(),
		entry.LastUpdated(),
	)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("write to database failed: %s and unable to rollback transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("write to database failed: %s", execErr)
	}
	execErr = tx.Commit()
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("write to database failed: %s and unable to rollback transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("write to database failed: %s", execErr)
	}
	return nil
}

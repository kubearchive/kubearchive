package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
)

const (
	apiVersionExtension = "apiversion"
	kindExtension       = "kind"
	nameExtension       = "name"
	namespaceExtension  = "namespace"
)

type Resource struct {
	Kind       string                 `json:"kind"`
	ApiVersion string                 `json:"apiVersion"`
	Spec       map[string]interface{} `json:"spec"`
	Status     map[string]interface{} `json:"status"`
	Metadata   map[string]interface{} `json:"metadata"`
}

func (r *Resource) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &r)
}

// Represents the fields in an event's metadata that need to be written to the database
type EventMetadata struct {
	ResourceVersion   string `json:"resourceVersion"`
	ClusterUid        string `json:"uid"`
	CreationTimestamp string `json:"creationTimestamp"`
	DeletionTimestamp string `json:"deletionTimestamp,omitempty"`
}

// Respresents the fields in a cloudevent that need to be written to the database
type EventData struct {
	Metadata EventMetadata `json:"metadata"`
}

// Represents an entry for a resource in the Database
type ResourceEntry struct {
	ApiVersion      string
	Cluster         string
	ClusterUid      string
	Created         string
	Data            []byte
	Deleted         sql.NullString
	Kind            string
	LastUpdated     string
	Name            string
	Namespace       string
	ResourceVersion string
}

// checks that the cloudevent has the appropriate extensions set and has values that are the right type. Additionally
// it checks that the cloudevent's data has all the necessary fields and returns a models.ResourceEntry. If all
// conditions are not met, it returns an error
func ResourceEntryFromCloudevent(event cloudevents.Event, cluster string) (*ResourceEntry, error) {
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
	timeStamp := event.Time().Format(time.RFC3339) // Kubernetes uses RFC 3339 format for time
	eventData := EventData{}
	err = event.DataAs(&eventData)
	if err != nil {
		return nil, err
	}

	return &ResourceEntry{
			ApiVersion: apiVersion,
			Cluster:    cluster,
			ClusterUid: eventData.Metadata.ClusterUid,
			Created:    eventData.Metadata.CreationTimestamp,
			Data:       event.Data(),
			Deleted: sql.NullString{
				String: eventData.Metadata.DeletionTimestamp,
				Valid:  eventData.Metadata.DeletionTimestamp != "",
			},
			Kind:            kind,
			LastUpdated:     timeStamp,
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: eventData.Metadata.ResourceVersion,
		},
		nil
}

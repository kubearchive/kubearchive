// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/huandu/go-sqlbuilder"
	dbErrors "github.com/kubearchive/kubearchive/pkg/database/errors"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	corev1 "k8s.io/api/core/v1"
)

func (db *sqlDatabaseImpl) QueryResourceByUID(ctx context.Context, kind, apiVersion, namespace, uid string) (*models.Resource, error) {
	sb := db.selector.ResourceSelector()
	sb.Where(
		db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
		db.filter.NamespaceFilter(sb.Cond, namespace),
		db.filter.UuidFilter(sb.Cond, uid))

	resources, err := db.performResourceQuery(ctx, sb)
	if err != nil {
		return nil, err
	}

	// QueryResources does not send an error if no resources are found
	// here we want to replicate the same behaviour.
	if len(resources) == 0 {
		return nil, nil //nolint:nilnil
	}

	return &resources[0], nil
}

// buildResourceListQuery builds the SQL select query for listing resources.
// This is shared between QueryResources and StreamResources.
func (db *sqlDatabaseImpl) buildResourceListQuery(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time, limit int) (*sqlbuilder.SelectBuilder, error) {
	sb := db.selector.ResourceSelector()
	sb.Where(db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion))
	if namespace != "" {
		sb.Where(db.filter.NamespaceFilter(sb.Cond, namespace))
	}

	isWildcardQuery := false
	if name != "" {
		if strings.Contains(name, "*") {
			sqlPattern := strings.ReplaceAll(name, "*", "%")
			sb.Where(db.filter.NameWildcardFilter(sb.Cond, sqlPattern))
			isWildcardQuery = true
		} else {
			sb.Where(db.filter.NameFilter(sb.Cond, name))
		}
	}

	if name == "" || isWildcardQuery {
		if continueId != "" && continueDate != "" {
			sb.Where(db.filter.CreationTSAndIDFilter(sb.Cond, continueDate, continueId))
		}
		if creationTimestampAfter != nil {
			sb.Where(db.filter.CreationTimestampAfterFilter(sb.Cond, *creationTimestampAfter))
		}
		if creationTimestampBefore != nil {
			sb.Where(db.filter.CreationTimestampBeforeFilter(sb.Cond, *creationTimestampBefore))
		}
		if !labelFilters.IsEmpty() {
			if err := db.filter.ApplyLabelFilters(ctx, db.db, sb, labelFilters); err != nil {
				return nil, err
			}
		}
		sb = db.sorter.CreationTSAndIDSorter(sb)
		sb.Limit(limit)
	}
	return sb, nil
}

func (db *sqlDatabaseImpl) QueryResources(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time, limit int) ([]models.Resource, error) {
	sb, err := db.buildResourceListQuery(ctx, kind, apiVersion, namespace, name,
		continueId, continueDate, labelFilters, creationTimestampAfter, creationTimestampBefore, limit)
	if err != nil {
		return nil, err
	}
	return db.performResourceQuery(ctx, sb)
}

func (db *sqlDatabaseImpl) StreamResources(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time, limit int,
	fn func(resource models.Resource) error) error {
	sb, err := db.buildResourceListQuery(ctx, kind, apiVersion, namespace, name,
		continueId, continueDate, labelFilters, creationTimestampAfter, creationTimestampBefore, limit)
	if err != nil {
		return err
	}
	return newQueryPerformer[models.Resource](db.db, db.flavor).performStreamQuery(ctx, sb, fn)
}

type uuidKindDate struct {
	Uuid string `db:"uuid"`
	Kind string `db:"kind"`
	Date string `db:"created_at"`
}

// Returns the log record and an error given a selector builder for a Pod
func (db *sqlDatabaseImpl) getLogsForPodSelector(ctx context.Context, sb *sqlbuilder.SelectBuilder, namespace, name, containerName string) (*interfaces.LogRecord, error) {
	type logURLRecord struct {
		URL   string         `db:"url"`
		Query sql.NullString `db:"query"`
		Start sql.NullString `db:"start"`
		End   sql.NullString `db:"end"`
	}
	logQueryPerformer := newQueryPerformer[logURLRecord](db.db, db.flavor)

	resources, err := db.performResourceQuery(ctx, sb)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve resource '%s/%s': %s", namespace, name, err.Error())
	}

	if len(resources) == 0 {
		return nil, dbErrors.ErrResourceNotFound
	}
	resource := resources[0]

	var pod corev1.Pod
	err = json.Unmarshal([]byte(resource.Data), &pod)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize pod '%s/%s': %s", namespace, name, err.Error())
	}

	if containerName == "" {
		annotations := pod.GetAnnotations()
		var ok bool
		containerName, ok = annotations[defaultContainerAnnotation]
		if !ok || containerName == "" {
			// This is to avoid index out of bounds error with no context
			if len(pod.Spec.Containers) == 0 {
				return nil, fmt.Errorf("pod '%s/%s' does not have containers, something went wrong", namespace, name)
			}
			containerName = pod.Spec.Containers[0].Name
		}
	}

	slog.DebugContext(
		ctx,
		"found pod preferred container for logs",
		"container", containerName,
		"namespace", namespace,
		"name", pod.Name,
	)
	sb = db.selector.UrlSelector()
	sb.Where(
		db.filter.UuidFilter(sb.Cond, resource.Uuid),
		db.filter.ContainerNameFilter(sb.Cond, containerName),
	)
	logUrls, err := logQueryPerformer.performQuery(ctx, sb)
	if err != nil {
		return nil, err
	}
	if len(logUrls) >= 1 {
		return &interfaces.LogRecord{
			Namespace:     namespace,
			PodName:       pod.Name,
			PodUUID:       resource.Uuid,
			ContainerName: containerName,
			URL:           logUrls[0].URL,
			Query:         logUrls[0].Query.String,
			Start:         logUrls[0].Start.String,
			End:           logUrls[0].End.String,
		}, nil
	} else {
		return nil, dbErrors.ErrResourceNotFound
	}
}

func (db *sqlDatabaseImpl) QueryLogURLByUID(ctx context.Context, kind, apiVersion, namespace, uid, containerName string) (*interfaces.LogRecord, error) {
	return db.queryLogURL(ctx, kind, apiVersion, namespace, uid, containerName, true)
}

func (db *sqlDatabaseImpl) QueryLogURLByName(ctx context.Context, kind, apiVersion, namespace, name, containerName string) (*interfaces.LogRecord, error) {
	return db.queryLogURL(ctx, kind, apiVersion, namespace, name, containerName, false)
}

func (db *sqlDatabaseImpl) queryLogURL(ctx context.Context, kind, apiVersion, namespace, identifier, containerName string, useUID bool) (*interfaces.LogRecord, error) {
	if kind == "Pod" {
		sb := db.selector.ResourceSelector()
		sb = db.sorter.CreationTSAndIDSorter(sb) // If resources are named the same, select the newest
		sb.Where(
			db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
			db.filter.NamespaceFilter(sb.Cond, namespace),
		)

		if useUID {
			sb.Where(db.filter.UuidFilter(sb.Cond, identifier))
		} else {
			sb.Where(db.filter.NameFilter(sb.Cond, identifier))
		}

		return db.getLogsForPodSelector(ctx, sb, namespace, identifier, containerName)
	}

	sb := db.selector.UUIDResourceSelector()
	sb = db.sorter.CreationTSAndIDSorter(sb) // If resources are named the same, select the newest
	sb.Where(
		db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
		db.filter.NamespaceFilter(sb.Cond, namespace),
	)

	if useUID {
		sb.Where(db.filter.UuidFilter(sb.Cond, identifier))
	} else {
		sb.Where(db.filter.NameFilter(sb.Cond, identifier))
	}

	strQueryPerformer := newQueryPerformer[string](db.db, db.flavor)
	uuid, err := strQueryPerformer.performSingleRowQuery(ctx, sb)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, dbErrors.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}

	slog.DebugContext(
		ctx,
		"getting owned pods for resource",
		"kind", kind,
		"namespace", namespace,
		"identifier", identifier,
		"uuid", uuid,
	)

	pods, err := db.getOwnedPodsUuids(ctx, []string{uuid}, []uuidKindDate{})
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, dbErrors.ErrResourceNotFound
	}

	slices.SortFunc(pods, func(a, b uuidKindDate) int {
		return strings.Compare(b.Date, a.Date)
	})

	sb = db.selector.ResourceSelector()
	sb.Where(db.filter.UuidFilter(sb.Cond, pods[0].Uuid))

	return db.getLogsForPodSelector(ctx, sb, namespace, uuid, containerName)
}

func (db *sqlDatabaseImpl) getOwnedPodsUuids(ctx context.Context, ownersUuids []string, podUuids []uuidKindDate,
) ([]uuidKindDate, error) {

	if len(ownersUuids) == 0 {
		return podUuids, nil
	} else {
		sb := db.selector.OwnedResourceSelector()
		sb.Where(db.filter.OwnerFilter(sb.Cond, ownersUuids))
		parsedRows, err := newQueryPerformer[uuidKindDate](db.db, db.flavor).performQuery(ctx, sb)
		if err != nil {
			return nil, err
		}
		ownersUuids = make([]string, 0)
		for _, row := range parsedRows {
			if row.Kind == "Pod" {
				podUuids = append(podUuids, row)
			} else {
				ownersUuids = append(ownersUuids, row.Uuid)
			}
		}
		return db.getOwnedPodsUuids(ctx, ownersUuids, podUuids)
	}
}

func (db *sqlDatabaseImpl) performResourceQuery(ctx context.Context, sb *sqlbuilder.SelectBuilder) ([]models.Resource, error) {

	resources, err := newQueryPerformer[models.Resource](db.db, db.flavor).performQuery(ctx, sb)
	if err != nil {
		return []models.Resource{}, err
	}

	return resources, nil
}

func (db *sqlDatabaseImpl) getSelector() facade.DBSelector {
	return db.selector
}

func (db *sqlDatabaseImpl) getSorter() facade.DBSorter {
	return db.sorter
}

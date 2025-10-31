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
	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	corev1 "k8s.io/api/core/v1"
)

func (db *sqlDatabaseImpl) QueryResourceByUID(ctx context.Context, uid string) (*models.Resource, error) {
	sb := db.selector.ResourceSelector()
	sb.Where(db.filter.UuidFilter(sb.Cond, uid))

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

func (db *sqlDatabaseImpl) QueryResources(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time, limit int) ([]models.Resource, error) {
	sb := db.selector.ResourceSelector()
	sb.Where(db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion))
	if namespace != "" {
		sb.Where(db.filter.NamespaceFilter(sb.Cond, namespace))
	}
	mainWhereClause := sqlbuilder.CopyWhereClause(sb.WhereClause)

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
		if labelFilters.Exists != nil {
			sb.Where(db.filter.ExistsLabelFilter(sb.Cond, labelFilters.Exists, mainWhereClause))
		}
		if labelFilters.NotExists != nil {
			sb.Where(db.filter.NotExistsLabelFilter(sb.Cond, labelFilters.NotExists, mainWhereClause))
		}
		if labelFilters.Equals != nil {
			sb.Where(db.filter.EqualsLabelFilter(sb.Cond, labelFilters.Equals, mainWhereClause))
		}
		if labelFilters.NotEquals != nil {
			sb.Where(db.filter.NotEqualsLabelFilter(sb.Cond, labelFilters.NotEquals, mainWhereClause))
		}
		if labelFilters.In != nil {
			sb.Where(db.filter.InLabelFilter(sb.Cond, labelFilters.In, mainWhereClause))
		}
		if labelFilters.NotIn != nil {
			sb.Where(db.filter.NotInLabelFilter(sb.Cond, labelFilters.NotIn, mainWhereClause))
		}
		sb = db.sorter.CreationTSAndIDSorter(sb)
		sb.Limit(limit)
	}
	return db.performResourceQuery(ctx, sb)
}

type uuidKindDate struct {
	Uuid string `db:"uuid"`
	Kind string `db:"kind"`
	Date string `db:"created_at"`
}

// Returns the log url, the json path and an error given a selector builder for a Pod
func (db *sqlDatabaseImpl) getLogsForPodSelector(ctx context.Context, sb *sqlbuilder.SelectBuilder, namespace, name, containerName string) (string, string, error) {
	type logURLJsonPath struct {
		LogURL   string `db:"url"`
		JsonPath string `db:"json_path"`
	}
	logQueryPerformer := newQueryPerformer[logURLJsonPath](db.db, db.flavor)

	resources, err := db.performResourceQuery(ctx, sb)
	if err != nil {
		return "", "", fmt.Errorf("could not retrieve resource '%s/%s': %s", namespace, name, err.Error())
	}

	if len(resources) == 0 {
		return "", "", dbErrors.ErrResourceNotFound
	}

	var pod corev1.Pod
	err = json.Unmarshal([]byte(resources[0].Data), &pod)
	if err != nil {
		return "", "", fmt.Errorf("failed to deserialize pod '%s/%s': %s", namespace, name, err.Error())
	}

	if containerName == "" {
		annotations := pod.GetAnnotations()
		var ok bool
		containerName, ok = annotations[defaultContainerAnnotation]
		if !ok || containerName == "" {
			// This is to avoid index out of bounds error with no context
			if len(pod.Spec.Containers) == 0 {
				return "", "", fmt.Errorf("pod '%s/%s' does not have containers, something went wrong", namespace, name)
			}
			containerName = pod.Spec.Containers[0].Name
		}
	}

	slog.DebugContext(
		ctx,
		"found pod preferred container for logs",
		"container", containerName,
		"namespace", namespace,
		"name", name,
	)
	sb = db.selector.UrlSelector()
	sb.Where(
		db.filter.UuidFilter(sb.Cond, string(pod.UID)),
		db.filter.ContainerNameFilter(sb.Cond, containerName),
	)
	logUrls, err := logQueryPerformer.performQuery(ctx, sb)
	if err != nil {
		return "", "", err
	}
	if len(logUrls) >= 1 {
		return logUrls[0].LogURL, logUrls[0].JsonPath, nil
	} else {
		return "", "", dbErrors.ErrResourceNotFound
	}
}

func (db *sqlDatabaseImpl) QueryLogURL(ctx context.Context, kind, apiVersion, namespace, identifier, containerName string, useUID bool) (string, string, error) {
	if kind == "Pod" {
		sb := db.selector.ResourceSelector()
		sb = db.sorter.CreationTSAndIDSorter(sb)
		if useUID {
			sb.Where(db.filter.UuidFilter(sb.Cond, identifier))
		} else {
			sb.Where(
				db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
				db.filter.NamespaceFilter(sb.Cond, namespace),
				db.filter.NameFilter(sb.Cond, identifier),
			)
		}

		return db.getLogsForPodSelector(ctx, sb, namespace, identifier, containerName)
	}

	sb := db.selector.UUIDResourceSelector()
	sb = db.sorter.CreationTSAndIDSorter(sb)
	if useUID {
		sb.Where(db.filter.UuidFilter(sb.Cond, identifier))
	} else {
		sb.Where(
			db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
			db.filter.NamespaceFilter(sb.Cond, namespace),
			db.filter.NameFilter(sb.Cond, identifier),
		)
	}

	strQueryPerformer := newQueryPerformer[string](db.db, db.flavor)
	uuid, err := strQueryPerformer.performSingleRowQuery(ctx, sb)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", dbErrors.ErrResourceNotFound
	}
	if err != nil {
		return "", "", err
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
		return "", "", err
	}
	if len(pods) == 0 {
		return "", "", dbErrors.ErrResourceNotFound
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
	type resourceFields struct {
		Date     string `db:"created_at"`
		Id       int64  `db:"id"`
		Resource string `db:"data"`
	}

	parsedRows, err := newQueryPerformer[resourceFields](db.db, db.flavor).performQuery(ctx, sb)
	var resources []models.Resource
	if err != nil {
		return resources, err
	}

	for _, parsedRow := range parsedRows {
		resources = append(resources, models.Resource{Date: parsedRow.Date, Id: parsedRow.Id, Data: parsedRow.Resource})
	}

	return resources, nil
}

func (db *sqlDatabaseImpl) getSelector() facade.DBSelector {
	return db.selector
}

func (db *sqlDatabaseImpl) getSorter() facade.DBSorter {
	return db.sorter
}

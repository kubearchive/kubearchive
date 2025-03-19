// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
	corev1 "k8s.io/api/core/v1"
)

type DBReader interface {
	Ping(ctx context.Context) error
	CloseDB() error

	setConn(conn *sqlx.DB)

	getFilter() facade.DBFilter
	getFlavor() sqlbuilder.Flavor
	getSelector() facade.DBSelector
	getSorter() facade.DBSorter

	QueryResources(ctx context.Context, kind, apiVersion, namespace,
		name, continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error)
	QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error)
}

func NewReader() (DBReader, error) {
	return getSingleDatabase()
}

func (db *DatabaseImpl) getSelector() facade.DBSelector {
	return db.selector
}

func (db *DatabaseImpl) getSorter() facade.DBSorter {
	return db.sorter
}

type uuidKindDate struct {
	Uuid string `db:"uuid"`
	Kind string `db:"kind"`
	Date string `db:"created_at"`
}

func (db *DatabaseImpl) QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error) {
	if kind == "Pod" {
		sb := db.getSelector().ResourceSelector()
		sb = db.getSorter().CreationTSAndIDSorter(sb) // If resources are named the same, select the newest
		sb.Where(
			db.filter.KindFilter(sb.Cond, kind),
			db.filter.ApiVersionFilter(sb.Cond, apiVersion),
			db.filter.NamespaceFilter(sb.Cond, namespace),
			db.filter.NameFilter(sb.Cond, name),
		)

		return db.getLogsForPodSelector(ctx, sb, namespace, name)
	}

	// Not a Pod
	sb := db.getSelector().UUIDResourceSelector()
	sb = db.getSorter().CreationTSAndIDSorter(sb) // If resources are named the same, select the newest
	sb.Where(
		db.filter.KindFilter(sb.Cond, kind),
		db.filter.ApiVersionFilter(sb.Cond, apiVersion),
		db.filter.NamespaceFilter(sb.Cond, namespace),
		db.filter.NameFilter(sb.Cond, name),
	)

	strQueryPerformer := newQueryPerformer[string](db.conn, db.flavor)
	uuid, err := strQueryPerformer.performSingleRowQuery(ctx, sb)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ResourceNotFoundError
	}

	if err != nil {
		return "", "", err
	}

	slog.Debug(
		"getting owned pods for resource",
		"kind", kind,
		"namespace", namespace,
		"name", name,
		"uuid", uuid,
	)
	pods, err := db.getOwnedPodsUuids(ctx, []string{uuid}, []uuidKindDate{})
	if err != nil {
		return "", "", err
	}
	if len(pods) == 0 {
		return "", "", ResourceNotFoundError
	}

	// Get the most recent pod from owned by the provided resource
	slices.SortFunc(pods, func(a, b uuidKindDate) int {
		return strings.Compare(b.Date, a.Date)
	})

	sb = db.getSelector().ResourceSelector()
	sb.Where(db.filter.UuidFilter(sb.Cond, pods[0].Uuid))

	return db.getLogsForPodSelector(ctx, sb, namespace, name)
}

func (db *DatabaseImpl) QueryResources(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error) {
	sb := db.getSelector().ResourceSelector()
	mainWhereClause := db.mainWhereClause(kind, apiVersion, namespace)
	sb.AddWhereClause(mainWhereClause)
	if name != "" {
		sb.Where(db.filter.NameFilter(sb.Cond, name))
	} else {
		if continueId != "" && continueDate != "" {
			sb.Where(db.filter.CreationTSAndIDFilter(sb.Cond, continueDate, continueId))
		}
		if labelFilters.Exists != nil {
			sb.Where(db.filter.ExistsLabelFilter(sb.Cond, labelFilters.Exists))
		}
		if labelFilters.NotExists != nil {
			sb.Where(db.filter.NotExistsLabelFilter(sb.Cond, labelFilters.NotExists))
		}
		if labelFilters.Equals != nil {
			sb.Where(db.filter.EqualsLabelFilter(sb.Cond, labelFilters.Equals))
		}
		if labelFilters.NotEquals != nil {
			sb.Where(db.filter.NotEqualsLabelFilter(sb.Cond, labelFilters.NotEquals, mainWhereClause))
		}
		if labelFilters.In != nil {
			sb.Where(db.filter.InLabelFilter(sb.Cond, labelFilters.In))
		}
		if labelFilters.NotIn != nil {
			sb.Where(db.filter.NotInLabelFilter(sb.Cond, labelFilters.NotIn))
		}
		sb = db.getSorter().CreationTSAndIDSorter(sb)
		sb.Limit(limit)
	}
	return db.performResourceQuery(ctx, sb)
}

func (db *DatabaseImpl) mainWhereClause(kind, apiVersion, namespace string) *sqlbuilder.WhereClause {
	whereClause := sqlbuilder.NewWhereClause()
	cond := sqlbuilder.NewCond()
	whereClause.AddWhereExpr(
		cond.Args,
		db.filter.KindFilter(*cond, kind),
		db.filter.ApiVersionFilter(*cond, apiVersion),
	)
	if namespace != "" {
		whereClause.AddWhereExpr(cond.Args, db.filter.NamespaceFilter(*cond, namespace))
	}
	return whereClause
}

// Returns the log url, the json path and an error given a Selector builder for a Pod
func (db *DatabaseImpl) getLogsForPodSelector(ctx context.Context, sb *sqlbuilder.SelectBuilder, namespace, name string) (string, string, error) {
	type logURLJsonPath struct {
		LogURL   string `db:"url"`
		JsonPath string `db:"json_path"`
	}
	logQueryPerformer := newQueryPerformer[logURLJsonPath](db.conn, db.flavor)

	podString, _, _, err := db.performResourceQuery(ctx, sb)
	if err != nil {
		return "", "", fmt.Errorf("could not retrieve resource '%s/%s': %s", namespace, name, err.Error())
	}

	if len(podString) == 0 {
		return "", "", ResourceNotFoundError
	}

	var pod corev1.Pod
	err = json.Unmarshal([]byte(podString[0]), &pod)
	if err != nil {
		return "", "", fmt.Errorf("failed to deserialize pod '%s/%s': %s", namespace, name, err.Error())
	}

	annotations := pod.GetAnnotations()
	containerName, ok := annotations[defaultContainerAnnotation]
	if !ok {
		// This is to avoid index out of bounds error with no context
		if len(pod.Spec.Containers) == 0 {
			return "", "", fmt.Errorf("pod '%s/%s' does not have containers, something went wrong", namespace, name)
		}
		containerName = pod.Spec.Containers[0].Name
	}

	slog.Debug(
		"found pod preferred container for logs",
		"container", containerName,
		"namespace", namespace,
		"name", name,
	)
	sb = db.getSelector().UrlSelector()
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
		return "", "", ResourceNotFoundError
	}
}

func (db *DatabaseImpl) getOwnedPodsUuids(ctx context.Context, ownersUuids []string, podUuids []uuidKindDate,
) ([]uuidKindDate, error) {

	if len(ownersUuids) == 0 {
		return podUuids, nil
	} else {
		sb := db.getSelector().OwnedResourceSelector()
		sb.Where(db.filter.OwnerFilter(sb.Cond, ownersUuids))
		parsedRows, err := newQueryPerformer[uuidKindDate](db.conn, db.flavor).performQuery(ctx, sb)
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

func (db *DatabaseImpl) performResourceQuery(ctx context.Context, sb *sqlbuilder.SelectBuilder) ([]string, int64, string, error) {
	type resourceFields struct {
		Date     string `db:"created_at"`
		Id       int64  `db:"id"`
		Resource string `db:"data"`
	}

	var resources []string

	parsedRows, err := newQueryPerformer[resourceFields](db.conn, db.flavor).performQuery(ctx, sb)

	if err != nil {
		return resources, 0, "", err
	}

	if len(parsedRows) == 0 {
		return resources, 0, "", err
	}
	lastRow := parsedRows[len(parsedRows)-1]

	for _, parsedRow := range parsedRows {
		resources = append(resources, parsedRow.Resource)
	}
	return resources, lastRow.Id, lastRow.Date, nil
}

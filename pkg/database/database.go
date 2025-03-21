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
	"sync"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

var ResourceNotFoundError = errors.New("resource not found")

type newDatabaseFunc func(*sqlx.DB) Database
type newDBCreatorFunc func(map[string]string) facade.DBCreator

var RegisteredDatabases = make(map[string]newDatabaseFunc)
var RegisteredDBCreators = make(map[string]newDBCreatorFunc)

type DBReader interface {
	QueryResources(ctx context.Context, kind, apiVersion, namespace,
		name, continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error)
	QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error)
	Ping(ctx context.Context) error
	CloseDB() error

	getSelector() facade.DBSelector
	getSorter() facade.DBSorter
}

type DBWriter interface {
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte, lastUpdated time.Time) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, jsonPath string, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error

	getInserter() facade.DBInserter
	getDeleter() facade.DBDeleter
}

type Database interface {
	DBReader
	DBWriter

	getFilter() facade.DBFilter
	getFlavor() sqlbuilder.Flavor
	setConn(*sqlx.DB)
}

type DatabaseImpl struct {
	db       *sqlx.DB
	flavor   sqlbuilder.Flavor
	selector facade.DBSelector
	filter   facade.DBFilter
	sorter   facade.DBSorter
	inserter facade.DBInserter
	deleter  facade.DBDeleter
}

var db Database
var once sync.Once

func NewReader() (DBReader, error) {
	return newDatabase()
}

func NewWriter() (DBWriter, error) {
	return newDatabase()
}

func newDatabase() (Database, error) {
	var err error

	once.Do(func() {
		env, newDBErr := newDatabaseEnvironment()
		if newDBErr != nil {
			err = newDBErr
			return
		}

		var creator facade.DBCreator
		if c, ok := RegisteredDBCreators[env[DbKindEnvVar]]; ok {
			creator = c(env)
		} else {
			err = fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
			return
		}

		conn, errConn := establishConnection(creator.GetDriverName(), creator.GetConnectionString())
		if errConn != nil {
			err = errConn
			return
		}

		if init, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
			db = init(conn)
		} else {
			err = fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
		}
	})

	return db, err
}

func (db *DatabaseImpl) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *DatabaseImpl) QueryResources(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error) {
	sb := db.selector.ResourceSelector()
	sb.Where(db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion))
	if namespace != "" {
		sb.Where(db.filter.NamespaceFilter(sb.Cond, namespace))
	}
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
			sb.Where(db.filter.NotEqualsLabelFilter(sb.Cond, labelFilters.NotEquals))
		}
		if labelFilters.In != nil {
			sb.Where(db.filter.InLabelFilter(sb.Cond, labelFilters.In))
		}
		if labelFilters.NotIn != nil {
			sb.Where(db.filter.NotInLabelFilter(sb.Cond, labelFilters.NotIn))
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
func (db *DatabaseImpl) getLogsForPodSelector(ctx context.Context, sb *sqlbuilder.SelectBuilder, namespace, name string) (string, string, error) {
	type logURLJsonPath struct {
		LogURL   string `db:"url"`
		JsonPath string `db:"json_path"`
	}
	logQueryPerformer := newQueryPerformer[logURLJsonPath](db.db, db.flavor)

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
		return "", "", ResourceNotFoundError
	}
}

func (db *DatabaseImpl) QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error) {
	if kind == "Pod" {
		sb := db.selector.ResourceSelector()
		sb = db.sorter.CreationTSAndIDSorter(sb) // If resources are named the same, select the newest
		sb.Where(
			db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
			db.filter.NamespaceFilter(sb.Cond, namespace),
			db.filter.NameFilter(sb.Cond, name),
		)

		return db.getLogsForPodSelector(ctx, sb, namespace, name)
	}

	// Not a Pod
	sb := db.selector.UUIDResourceSelector()
	sb = db.sorter.CreationTSAndIDSorter(sb) // If resources are named the same, select the newest
	sb.Where(
		db.filter.KindApiVersionFilter(sb.Cond, kind, apiVersion),
		db.filter.NamespaceFilter(sb.Cond, namespace),
		db.filter.NameFilter(sb.Cond, name),
	)

	strQueryPerformer := newQueryPerformer[string](db.db, db.flavor)
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

	sb = db.selector.ResourceSelector()
	sb.Where(db.filter.UuidFilter(sb.Cond, pods[0].Uuid))

	return db.getLogsForPodSelector(ctx, sb, namespace, name)
}

func (db *DatabaseImpl) getOwnedPodsUuids(ctx context.Context, ownersUuids []string, podUuids []uuidKindDate,
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

func (db *DatabaseImpl) performResourceQuery(ctx context.Context, sb *sqlbuilder.SelectBuilder) ([]string, int64, string, error) {
	type resourceFields struct {
		Date     string `db:"created_at"`
		Id       int64  `db:"id"`
		Resource string `db:"data"`
	}

	var resources []string

	parsedRows, err := newQueryPerformer[resourceFields](db.db, db.flavor).performQuery(ctx, sb)

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

func (db *DatabaseImpl) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte, lastUpdated time.Time) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	query, args := db.inserter.ResourceInserter(
		string(k8sObj.GetUID()),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		lastUpdated,
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
	).BuildWithFlavor(db.flavor)
	_, execErr := tx.ExecContext(
		ctx,
		query,
		args...,
	)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("write to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("write to database failed: %s", execErr)
	}
	execErr = tx.Commit()
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("commit to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("commit to database failed and the transactions was rolled back: %s", execErr)
	}
	return nil
}

// WriteUrls deletes urls for k8sObj before writing urls to prevent duplicates. If logs is empty or nil all urls for
// k8sObj will be deleted from the database and will not be replaced
func (db *DatabaseImpl) WriteUrls(
	ctx context.Context,
	k8sObj *unstructured.Unstructured,
	jsonPath string,
	logs ...models.LogTuple,
) error {
	// The sink performs checks before WriteUrls is called, which currently make it not possible for this check to
	// evaluate to true during normal program execution. This check is here purely as a safeguard.
	if k8sObj == nil {
		return errors.New("Cannot write log urls to the database when k8sObj is nil")
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf(
			"could not begin transaction to write urls for resource %s: %w",
			string(k8sObj.GetUID()),
			err,
		)
	}
	delBuilder := db.deleter.UrlDeleter()
	delBuilder.Where(db.filter.UuidFilter(delBuilder.Cond, string(k8sObj.GetUID())))
	query, args := delBuilder.BuildWithFlavor(db.flavor)
	_, execErr := tx.ExecContext(ctx, query, args...)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf(
				"delete to database failed: %w and unable to roll back transaction: %w",
				execErr,
				rollbackErr,
			)
		}
		return fmt.Errorf("delete to database failed: %w", execErr)
	}

	for _, log := range logs {
		logQuery, logArgs := db.inserter.UrlInserter(
			string(k8sObj.GetUID()),
			log.Url,
			log.ContainerName,
			jsonPath,
		).BuildWithFlavor(db.flavor)
		_, logQueryErr := tx.ExecContext(ctx, logQuery, logArgs...)
		if logQueryErr != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				return fmt.Errorf(
					"write to database failed: %w and unable to roll back transaction: %w",
					execErr,
					rollbackErr,
				)
			}
			return fmt.Errorf("write to database failed: %w", execErr)
		}
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf(
				"commit to database failed: %w and unable rollback transaction: %w",
				commitErr,
				rollbackErr,
			)
		}
		return fmt.Errorf("commit to database failed and the transaction was rolled back: %w", commitErr)
	}
	return nil
}

func (db *DatabaseImpl) CloseDB() error {
	return db.db.Close()
}

func (db *DatabaseImpl) getSelector() facade.DBSelector {
	return db.selector
}

func (db *DatabaseImpl) getFilter() facade.DBFilter {
	return db.filter
}

func (db *DatabaseImpl) getInserter() facade.DBInserter {
	return db.inserter
}

func (db *DatabaseImpl) getSorter() facade.DBSorter {
	return db.sorter
}

func (db *DatabaseImpl) getDeleter() facade.DBDeleter {
	return db.deleter
}

func (db *DatabaseImpl) getFlavor() sqlbuilder.Flavor {
	return db.flavor
}

func (db *DatabaseImpl) setConn(conn *sqlx.DB) {
	db.db = conn
}

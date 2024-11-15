// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var ResourceNotFoundError = errors.New("resource not found")

type newDatabaseFunc func(*sqlx.DB) DBInterface
type newDBCreatorFunc func(map[string]string) DBCreator

var RegisteredDatabases = make(map[string]newDatabaseFunc)
var RegisteredDBCreators = make(map[string]newDBCreatorFunc)

type Selector string
type Filter string
type Sorter string
type Limiter string

type DBCreator interface {
	GetDriverName() string
	GetConnectionString() string
}

// DBSelector encapsulates all the selector functions that must be implemented by the drivers
type DBSelector interface {
	ResourceSelector() Selector
	UUIDResourceSelector() Selector
	OwnedResourceSelector() Selector
	UrlFromResourceSelector() Selector
	UrlSelector() Selector
}

// DBFilter encapsulates all the filter functions that must be implemented by the drivers
// All its functions share the same signature
type DBFilter interface {
	PodFilter(idx int) (Filter, int)
	KindFilter(idx int) (Filter, int)
	ApiVersionFilter(idx int) (Filter, int)
	NamespaceFilter(idx int) (Filter, int)
	NameFilter(idx int) (Filter, int)
	CreationTSAndIDFilter(idx int) (Filter, int)
	OwnerFilter(idx int) (Filter, int)
	UuidFilter(idx int) (Filter, int)
}

// DBSorter encapsulates all the sorter functions that must be implemented by the drivers
type DBSorter interface {
	CreationTSAndIDSorter() Sorter
}

// DBLimiter encapsulates all the limiter functions that must be implemented by the drivers
type DBLimiter interface {
	Limiter(idx int) Limiter
}

// DBInserter encapsulates all the writer functions that must be implemented by the drivers
type DBInserter interface {
	ResourceInserter() string
	UrlInserter() string
}

// DBDeleter encapsulates all the deletion functions that must be implemented by the drivers
type DBDeleter interface {
	UrlDeleter() string
}

// DBParamParser need to be implemented by every database driver compatible with KubeArchive
type DBParamParser interface {
	// ParseParams transform the given args to accepted parameters for parametrized queries
	ParseParams(query string, args ...any) (string, []any, error)
}

type DBInterface interface {
	QueryResources(ctx context.Context, kind, apiVersion, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error)
	QueryNamespacedResources(ctx context.Context, kind, apiVersion, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error)
	QueryNamespacedResourceByName(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error)
	QueryLogURLs(ctx context.Context, kind, apiVersion, namespace, name string) ([]string, error)

	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error
}

type Database struct {
	db          *sqlx.DB
	selector    DBSelector
	filter      DBFilter
	sorter      DBSorter
	limiter     DBLimiter
	inserter    DBInserter
	deleter     DBDeleter
	paramParser DBParamParser
}

func NewDatabase() (DBInterface, error) {
	env, err := newDatabaseEnvironment()
	if err != nil {
		return nil, err
	}

	var creator DBCreator
	if c, ok := RegisteredDBCreators[env[DbKindEnvVar]]; ok {
		creator = c(env)
	} else {
		panic(fmt.Sprintf("No database registered with name %s", env[DbKindEnvVar]))
	}

	conn := establishConnection(creator.GetDriverName(), creator.GetConnectionString())

	var database DBInterface
	if init, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
		database = init(conn)
	} else {
		panic(fmt.Sprintf("No database registered with name %s", env[DbKindEnvVar]))
	}

	return database, nil
}

func (db *Database) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *Database) QueryResources(ctx context.Context, kind, apiVersion, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	pq := db.newParamQuery(db.selector.ResourceSelector())
	pq.addFilters(db.filter.KindFilter, db.filter.ApiVersionFilter)
	pq.addStringParams(kind, apiVersion)

	if continueId != "" && continueDate != "" {
		pq.addFilters(db.filter.CreationTSAndIDFilter)
		pq.addStringParams(continueDate, continueId)
	}

	pq.sorter = db.sorter.CreationTSAndIDSorter()
	pq.setLimiter(db.limiter.Limiter)
	pq.addStringParams(limit)

	return db.performResourceQuery(ctx, pq)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, apiVersion, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	pq := db.newParamQuery(db.selector.ResourceSelector())
	pq.addFilters(db.filter.KindFilter, db.filter.ApiVersionFilter, db.filter.NamespaceFilter)
	pq.addStringParams(kind, apiVersion, namespace)

	if continueId != "" && continueDate != "" {
		pq.addFilters(db.filter.CreationTSAndIDFilter)
		pq.addStringParams(continueDate, continueId)
	}

	pq.sorter = db.sorter.CreationTSAndIDSorter()
	pq.setLimiter(db.limiter.Limiter)
	pq.addStringParams(limit)

	return db.performResourceQuery(ctx, pq)
}

func (db *Database) QueryNamespacedResourceByName(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error) {
	pq := db.newParamQuery(db.selector.ResourceSelector())
	pq.addFilters(db.filter.KindFilter, db.filter.ApiVersionFilter, db.filter.NamespaceFilter, db.filter.NameFilter)
	pq.addStringParams(kind, apiVersion, namespace, name)
	resources, _, _, err := db.performResourceQuery(ctx, pq)
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, err
	} else if len(resources) == 1 {
		return resources[0], err
	} else {
		return nil, fmt.Errorf("more than one resource found")
	}
}

func (db *Database) QueryLogURLs(ctx context.Context, kind, apiVersion, namespace, name string) ([]string, error) {

	strQueryPerformer := newQueryPerformer[string](db.db)
	if kind == "Pod" {
		pq := db.newParamQuery(db.selector.UrlFromResourceSelector())
		pq.addFilters(db.filter.PodFilter, db.filter.ApiVersionFilter, db.filter.NamespaceFilter, db.filter.NameFilter)
		pq.addStringParams(apiVersion, namespace, name)
		return strQueryPerformer.performQuery(ctx, pq)
	}

	pq := db.newParamQuery(db.selector.UUIDResourceSelector())
	pq.addFilters(db.filter.KindFilter, db.filter.ApiVersionFilter, db.filter.NamespaceFilter, db.filter.NameFilter)
	pq.addStringParams(kind, apiVersion, namespace, name)
	uuid, err := strQueryPerformer.performSingleRowQuery(ctx, pq)

	if errors.Is(err, sql.ErrNoRows) {
		return []string{}, ResourceNotFoundError
	}

	if err != nil {
		return nil, err
	}

	podUuids, err := db.getOwnedPodsUuids(ctx, []string{uuid}, []string{})
	if err != nil {
		return nil, err
	}

	pq = db.newParamQuery(db.selector.UrlSelector())
	pq.addFilters(db.filter.UuidFilter)
	pq.addStringArrayParam(podUuids)
	return strQueryPerformer.performQuery(ctx, pq)
}

func (db *Database) getOwnedPodsUuids(ctx context.Context, ownersUuids, podUuids []string) ([]string, error) {

	type uuidKind struct {
		Uuid string
		Kind string
	}

	if len(ownersUuids) == 0 {
		return podUuids, nil
	} else {
		pq := db.newParamQuery(db.selector.OwnedResourceSelector())
		pq.addFilters(db.filter.OwnerFilter)
		pq.addStringArrayParam(ownersUuids)
		parsedRows, err := newQueryPerformer[uuidKind](db.db).performQuery(ctx, pq)
		if err != nil {
			return nil, err
		}
		ownersUuids = make([]string, 0)
		for _, row := range parsedRows {
			if row.Kind == "Pod" {
				podUuids = append(podUuids, row.Uuid)
			} else {
				ownersUuids = append(ownersUuids, row.Uuid)
			}
		}
		return db.getOwnedPodsUuids(ctx, ownersUuids, podUuids)
	}
}

func (db *Database) performResourceQuery(ctx context.Context, pq *paramQuery) ([]*unstructured.Unstructured, int64, string, error) {
	type resourceFields struct {
		Date     string `db:"created_at"`
		Id       int64  `db:"id"`
		Resource []byte `db:"data"`
	}

	var resources []*unstructured.Unstructured

	parsedRows, err := newQueryPerformer[resourceFields](db.db).performQuery(ctx, pq)

	if err != nil {
		return resources, 0, "", err
	}

	if len(parsedRows) == 0 {
		return resources, 0, "", err
	}
	lastRow := parsedRows[len(parsedRows)-1]

	for _, parsedRow := range parsedRows {
		r, errConversion := models.UnstructuredFromByteSlice(parsedRow.Resource)
		if errConversion != nil {
			return resources, 0, "", errConversion
		}
		resources = append(resources, r)
	}
	return resources, lastRow.Id, lastRow.Date, nil
}

func (db *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		db.inserter.ResourceInserter(),
		k8sObj.GetUID(),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
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

// WriteUrls deletes urls for k8sObj before writing urls to prevent duplicates
func (db *Database) WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, logs ...models.LogTuple) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf(
			"could not begin transaction to write urls for resource %s: %w",
			k8sObj.GetUID(),
			err,
		)
	}
	_, execErr := tx.ExecContext(
		ctx,
		db.deleter.UrlDeleter(),
		k8sObj.GetUID(),
	)
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
		_, execErr := tx.ExecContext(
			ctx,
			db.inserter.UrlInserter(),
			k8sObj.GetUID(),
			log.Url,
			log.ContainerName,
		)
		if execErr != nil {
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

func (db *Database) newParamQuery(selector Selector) *paramQuery {
	return &paramQuery{
		selector:      selector,
		dbArrayParser: db.paramParser}
}

func (db *Database) CloseDB() error {
	return db.db.Close()
}

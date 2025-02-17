// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var ResourceNotFoundError = errors.New("resource not found")

type newDatabaseFunc func(*sqlx.DB) DBInterface
type newDBCreatorFunc func(map[string]string) facade.DBCreator

var RegisteredDatabases = make(map[string]newDatabaseFunc)
var RegisteredDBCreators = make(map[string]newDBCreatorFunc)

type DBInterface interface {
	QueryResources(ctx context.Context, kind, apiVersion, namespace,
		name, continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error)
	QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error)

	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, jsonPath string, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error
}

type Database struct {
	DB       *sqlx.DB
	Flavor   sqlbuilder.Flavor
	Selector facade.DBSelector
	Filter   facade.DBFilter
	Sorter   facade.DBSorter
	Inserter facade.DBInserter
	Deleter  facade.DBDeleter
}

func NewDatabase() (DBInterface, error) {
	env, err := newDatabaseEnvironment()
	if err != nil {
		return nil, err
	}

	var creator facade.DBCreator
	if c, ok := RegisteredDBCreators[env[DbKindEnvVar]]; ok {
		creator = c(env)
	} else {
		return nil, fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
	}

	conn, err := establishConnection(creator.GetDriverName(), creator.GetConnectionString())
	if err != nil {
		return nil, err
	}

	var database DBInterface
	if init, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
		database = init(conn)
	} else {
		return nil, fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
	}

	return database, nil
}

func (db *Database) Ping(ctx context.Context) error {
	return db.DB.PingContext(ctx)
}

func (db *Database) QueryResources(ctx context.Context, kind, apiVersion, namespace, name,
	continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error) {
	sb := db.Selector.ResourceSelector()
	sb.Where(
		db.Filter.KindFilter(sb.Cond, kind),
		db.Filter.ApiVersionFilter(sb.Cond, apiVersion),
	)

	if namespace != "" {
		sb.Where(db.Filter.NamespaceFilter(sb.Cond, namespace))
	}
	if name != "" {
		sb.Where(db.Filter.NameFilter(sb.Cond, name))
	} else {
		if continueId != "" && continueDate != "" {
			sb.Where(db.Filter.CreationTSAndIDFilter(sb.Cond, continueDate, continueId))
		}
		if labelFilters.Exists != nil {
			sb.Where(db.Filter.ExistsLabelFilter(sb.Cond, labelFilters.Exists))
		}
		if labelFilters.NotExists != nil {
			sb.Where(db.Filter.NotExistsLabelFilter(sb.Cond, labelFilters.NotExists))
		}
		if labelFilters.Equals != nil {
			sb.Where(db.Filter.EqualsLabelFilter(sb.Cond, labelFilters.Equals))
		}
		if labelFilters.NotEquals != nil {
			sb.Where(db.Filter.NotEqualsLabelFilter(sb.Cond, labelFilters.NotEquals))
		}
		if labelFilters.In != nil {
			sb.Where(db.Filter.InLabelFilter(sb.Cond, labelFilters.In))
		}
		if labelFilters.NotIn != nil {
			sb.Where(db.Filter.NotInLabelFilter(sb.Cond, labelFilters.NotIn))
		}
		sb = db.Sorter.CreationTSAndIDSorter(sb)
		sb.Limit(limit)
	}
	return db.performResourceQuery(ctx, sb)
}

type uuidKindDate struct {
	Uuid string `db:"uuid"`
	Kind string `db:"kind"`
	Date string `db:"created_at"`
}

func (db *Database) QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error) {

	type logURLJsonPath struct {
		LogURL   string `db:"url"`
		JsonPath string `db:"json_path"`
	}
	strQueryPerformer := newQueryPerformer[string](db.DB, db.Flavor)
	logQueryPerformer := newQueryPerformer[logURLJsonPath](db.DB, db.Flavor)
	if kind == "Pod" {
		sb := db.Selector.UrlFromResourceSelector()
		sb.Where(
			db.Filter.KindFilter(sb.Cond, kind),
			db.Filter.ApiVersionFilter(sb.Cond, apiVersion),
			db.Filter.NamespaceFilter(sb.Cond, namespace),
			db.Filter.NameFilter(sb.Cond, name),
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

	sb := db.Selector.UUIDResourceSelector()
	sb.Where(
		db.Filter.KindFilter(sb.Cond, kind),
		db.Filter.ApiVersionFilter(sb.Cond, apiVersion),
		db.Filter.NamespaceFilter(sb.Cond, namespace),
		db.Filter.NameFilter(sb.Cond, name),
	)
	uuid, err := strQueryPerformer.performSingleRowQuery(ctx, sb)

	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ResourceNotFoundError
	}

	if err != nil {
		return "", "", err
	}

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

	sb = db.Selector.UrlSelector()
	sb.Where(db.Filter.UuidsFilter(sb.Cond, []string{pods[0].Uuid}))
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

func (db *Database) getOwnedPodsUuids(ctx context.Context, ownersUuids []string, podUuids []uuidKindDate,
) ([]uuidKindDate, error) {

	if len(ownersUuids) == 0 {
		return podUuids, nil
	} else {
		sb := db.Selector.OwnedResourceSelector()
		sb.Where(db.Filter.OwnerFilter(sb.Cond, ownersUuids))
		parsedRows, err := newQueryPerformer[uuidKindDate](db.DB, db.Flavor).performQuery(ctx, sb)
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

func (db *Database) performResourceQuery(ctx context.Context, sb *sqlbuilder.SelectBuilder) ([]string, int64, string, error) {
	type resourceFields struct {
		Date     string `db:"created_at"`
		Id       int64  `db:"id"`
		Resource string `db:"data"`
	}

	var resources []string

	parsedRows, err := newQueryPerformer[resourceFields](db.DB, db.Flavor).performQuery(ctx, sb)

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

func (db *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	query, args := db.Inserter.ResourceInserter(
		string(k8sObj.GetUID()),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
	).BuildWithFlavor(db.Flavor)
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
func (db *Database) WriteUrls(
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

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf(
			"could not begin transaction to write urls for resource %s: %w",
			string(k8sObj.GetUID()),
			err,
		)
	}
	delBuilder := db.Deleter.UrlDeleter()
	delBuilder.Where(db.Filter.UuidFilter(delBuilder.Cond, string(k8sObj.GetUID())))
	query, args := delBuilder.BuildWithFlavor(db.Flavor)
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
		logQuery, logArgs := db.Inserter.UrlInserter(
			string(k8sObj.GetUID()),
			log.Url,
			log.ContainerName,
			jsonPath,
		).BuildWithFlavor(db.Flavor)
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

func (db *Database) CloseDB() error {
	return db.DB.Close()
}

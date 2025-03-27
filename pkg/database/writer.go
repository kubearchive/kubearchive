// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kronicler/kronicler/pkg/database/facade"
	"github.com/kronicler/kronicler/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DBWriter interface {
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte, lastUpdated time.Time) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, jsonPath string, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error

	getInserter() facade.DBInserter
	getDeleter() facade.DBDeleter
	getFilter() facade.DBFilter
	getFlavor() sqlbuilder.Flavor
	setConn(*sqlx.DB)
}

func NewWriter() (DBWriter, error) {
	return newDatabase()
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

func (db *DatabaseImpl) getInserter() facade.DBInserter {
	return db.inserter
}

func (db *DatabaseImpl) getDeleter() facade.DBDeleter {
	return db.deleter
}

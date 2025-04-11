// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WriteUrls deletes urls for k8sObj before writing urls to prevent duplicates. If logs is empty or nil all urls for
// k8sObj will be deleted from the database and will not be replaced
func (db *sqlDatabaseImpl) WriteUrls(
	ctx context.Context,
	k8sObj *unstructured.Unstructured,
	jsonPath string,
	logs ...models.LogTuple,
) error {
	// The sink performs checks before WriteUrls is called, which currently make it not possible for this check to
	// evaluate to true during normal program execution. This check is here purely as a safeguard.
	if k8sObj == nil {
		return errors.New("cannot write log urls to the database when k8sObj is nil")
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

func (db *sqlDatabaseImpl) getInserter() facade.DBInserter {
	return db.inserter
}

func (db *sqlDatabaseImpl) getDeleter() facade.DBDeleter {
	return db.deleter
}

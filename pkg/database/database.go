// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const DefaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

var ResourceNotFoundError = errors.New("resource not found")

var RegisteredDatabases = make(map[string]DBInterface)

type DBInterface interface {
	QueryResources(ctx context.Context, kind, apiVersion, namespace,
		name, continueId, continueDate string, labelFilters *LabelFilters, limit int) ([]string, int64, string, error)
	QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name string) (string, string, error)

	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, jsonPath string, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error

	Init(env map[string]string) error
}

func NewDatabase() (DBInterface, error) {
	env, err := newDatabaseEnvironment()
	if err != nil {
		return nil, err
	}

	var database DBInterface
	if db, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
		err = db.Init(env)
		if err != nil {
			return nil, err
		}
		database = db
	} else {
		return nil, fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
	}

	return database, nil
}

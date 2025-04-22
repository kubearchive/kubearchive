// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package interfaces

import (
	"context"
	"time"

	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DBReader interface {
	QueryResources(ctx context.Context, kind, apiVersion, namespace,
		name, continueId, continueDate string, labelFilters *models.LabelFilters, limit int) ([]string, int64, string, error)
	QueryLogURL(ctx context.Context, kind, apiVersion, namespace, name, containerName string) (string, string, error)
	Ping(ctx context.Context) error
	QueryDatabaseSchemaVersion(ctx context.Context) (string, error)
	CloseDB() error
	Init(env map[string]string) error
}

type DBWriter interface {
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte, lastUpdated time.Time) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, jsonPath string, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	QueryDatabaseSchemaVersion(ctx context.Context) (string, error)
	CloseDB() error
	Init(env map[string]string) error
}

type Database interface {
	DBReader
	DBWriter
}

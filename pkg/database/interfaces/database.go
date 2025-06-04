// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package interfaces

import (
	"context"
	"time"

	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type WriteResourceResult uint

const (
	WriteResourceResultInserted WriteResourceResult = iota
	WriteResourceResultUpdated
	WriteResourceResultNone
	WriteResourceResultError
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
	// WriteResource writes the logs (when the resource is a Pod) and the resource into their respective tables
	// The log entries related to the resource are deleted first to prevent duplicates
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte, lastUpdated time.Time, jsonPath string, logs ...models.LogTuple) (WriteResourceResult, error)
	Ping(ctx context.Context) error
	QueryDatabaseSchemaVersion(ctx context.Context) (string, error)
	CloseDB() error
	Init(env map[string]string) error
}

type Database interface {
	DBReader
	DBWriter
}

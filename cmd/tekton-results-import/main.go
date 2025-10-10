// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	otelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	ce "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	_ "github.com/lib/pq"

	"github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/constants"
)

const (
	tektonResultsImportSource = "kubearchive.org/tekton-results-import"
)

type RecordCounts struct {
	kinds      map[string]int64
	totalCount int64
}

type ImportCloudEventSender struct {
	httpClient client.Client
	target     string
}

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	dbPort := os.Getenv("DATABASE_PORT")
	dbUser := os.Getenv("DATABASE_USER")
	dbPassword := os.Getenv("DATABASE_PASSWORD")
	dbName := os.Getenv("DATABASE_DB")

	if dbURL == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
		slog.Error("Missing required database environment variables")
		os.Exit(1)
	}

	port, err := strconv.Atoi(dbPort)
	if err != nil {
		slog.Error("Invalid DATABASE_PORT", "port", dbPort, "error", err)
		os.Exit(1)
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable connect_timeout=30",
		dbURL, port, dbUser, dbPassword, dbName)

	slog.Info("Connecting to database", "host", dbURL, "port", port, "user", dbUser, "database", dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("Failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Configure connection pool for long-running import operation
	db.SetMaxOpenConns(1)    // Single connection for sequential processing
	db.SetMaxIdleConns(1)    // Keep one connection alive
	db.SetConnMaxLifetime(0) // No max lifetime for long-running batch processing

	if err = db.Ping(); err != nil {
		slog.Error("Failed to ping database", "error", err)
		os.Exit(1)
	}

	slog.Info("Successfully connected to database")

	sender, err := NewImportCloudEventSender()
	if err != nil {
		slog.Error("Failed to create cloud event sender", "error", err)
		os.Exit(1)
	}

	slog.Info("Successfully initialized cloud event sender")

	counts, err := processRecords(ctx, db, sender)
	if err != nil {
		slog.Error("Failed to process records", "error", err)
		os.Exit(1)
	}

	slog.Info("=== FINAL IMPORT STATISTICS ===")
	slog.Info("Records processed by kind:")
	for kind, count := range counts.kinds {
		slog.Info("Kind processed", "kind", kind, "count", count)
	}
	slog.Info("Total records processed", "total", counts.totalCount)
	slog.Info("Import completed successfully")
}

func processRecords(ctx context.Context, db *sql.DB, sender *ImportCloudEventSender) (*RecordCounts, error) {
	query := "SELECT data FROM records WHERE data->>'kind' <> 'Log'"

	slog.Info("Executing query to retrieve records")

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	counts := &RecordCounts{
		kinds: make(map[string]int64),
	}

	slog.Info("Starting to process records")

	for rows.Next() {
		var dataJSON string
		if err = rows.Scan(&dataJSON); err != nil {
			slog.Error("Failed to scan row", "error", err)
			continue
		}

		var resource map[string]interface{}
		if err = json.Unmarshal([]byte(dataJSON), &resource); err != nil {
			slog.Error("Failed to unmarshal record JSON", "error", err)
			continue
		}

		kind := "unknown"
		if kindVal, ok := resource["kind"]; ok {
			if kindStr, ok := kindVal.(string); ok {
				kind = kindStr
			}
		}

		if err = sender.SendCloudEvent(ctx, resource); err != nil {
			slog.Error("Failed to send cloud event", "kind", kind, "error", err)
		}

		counts.kinds[kind]++
		counts.totalCount++

		if counts.totalCount%1000 == 0 {
			slog.Info("Progress update", "records_processed", counts.totalCount)
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error occurred during row iteration: %w", err)
	}

	return counts, nil
}

func NewImportCloudEventSender() (*ImportCloudEventSender, error) {
	sender := &ImportCloudEventSender{}

	var err error
	if sender.httpClient, err = otelObs.NewClientHTTP([]cehttp.Option{}, []client.Option{}); err != nil {
		return nil, fmt.Errorf("failed to create cloud event HTTP client: %w", err)
	}

	sender.target = cloudevents.GetSinkServiceUrl()

	return sender, nil
}

func (s *ImportCloudEventSender) SendCloudEvent(ctx context.Context, resource map[string]interface{}) error {
	event := ce.NewEvent()
	event.SetSource(tektonResultsImportSource)
	event.SetType(constants.TektonResultsImportEventType)

	if err := event.SetData(ce.ApplicationJSON, resource); err != nil {
		return fmt.Errorf("error setting cloudevent data: %w", err)
	}

	if apiVersion, ok := resource["apiVersion"].(string); ok {
		event.SetExtension("apiversion", apiVersion)
	}
	if kind, ok := resource["kind"].(string); ok {
		event.SetExtension("kind", kind)
	}

	if metadata, ok := resource["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			event.SetExtension("name", name)
		}
		if namespace, ok := metadata["namespace"].(string); ok {
			event.SetExtension("namespace", namespace)
		}
	}

	ectx := ce.ContextWithTarget(ctx, s.target)
	sendResult := s.httpClient.Send(ectx, event)

	if ce.IsACK(sendResult) {
		return nil
	}

	var httpResult *cehttp.Result
	statusCode := 0
	if ce.ResultAs(sendResult, &httpResult) {
		statusCode = httpResult.StatusCode
	}

	return fmt.Errorf("failed to send cloud event, status code: %d, error: %v", statusCode, sendResult)
}

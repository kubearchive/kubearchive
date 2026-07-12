// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"context"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var (
	ErrResourceNotFound = errors.New("resource not found")
	ErrQueryTimeout     = errors.New("context deadline exceeded")
)

const pgQueryCancelled = "57014"

// WrapQueryError wraps database query errors to detect timeout conditions.
// Returns ErrQueryTimeout if the error was caused by context deadline or PostgreSQL query cancellation.
func WrapQueryError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// Check context error first in case query was cancelled
	if ctxErr := ctx.Err(); ctxErr == context.Canceled || ctxErr == context.DeadlineExceeded {
		return fmt.Errorf("%w: %w", ErrQueryTimeout, ctxErr)
	}

	// Check if context deadline was exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrQueryTimeout, err)
	}

	// Check for PostgreSQL error code 57014 (query_canceled)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == pgQueryCancelled {
		return fmt.Errorf("%w: %w", ErrQueryTimeout, pqErr)
	}

	return err
}

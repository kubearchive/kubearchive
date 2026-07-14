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
	ErrResourceNotFound    = errors.New("resource not found")
	ErrQueryTimeout        = errors.New("query timeout")
	ErrContextQueryTimeout = fmt.Errorf("context deadline exceeded: %w", ErrQueryTimeout)
	ErrDatabaseTimeout     = fmt.Errorf("database query timeout: %w", ErrQueryTimeout)
	ErrContextCancelled    = errors.New("user context cancelled")
)

const pgQueryCancelled = "57014"

// WrapQueryError wraps database query errors to detect timeout conditions.
// Returns ErrQueryTimeout if the error was caused by context deadline or PostgreSQL query cancellation.
func WrapQueryError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// Check context error first in case query was cancelled
	if ctxErr := ctx.Err(); ctxErr == context.Canceled {
		return fmt.Errorf("%w: %w", ErrContextCancelled, ctxErr)
	} else if ctxErr == context.DeadlineExceeded {
		return fmt.Errorf("%w: %w", ErrContextQueryTimeout, ctxErr)
	}

	// Check if context deadline was exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrDatabaseTimeout, err)
	}

	// Check for PostgreSQL error code 57014 (query_canceled)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == pgQueryCancelled {
		return fmt.Errorf("%w: %w", ErrContextQueryTimeout, pqErr)
	}

	return err
}

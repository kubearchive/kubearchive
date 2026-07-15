// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"context"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
)

var (
	ErrResourceNotFound    = errors.New("resource not found")
	ErrQueryTimeout        = errors.New("query timeout")
	ErrContextQueryTimeout = fmt.Errorf("context deadline exceeded: %w", ErrQueryTimeout)
	ErrDatabaseTimeout     = fmt.Errorf("database query timeout: %w", ErrQueryTimeout)
	ErrContextCancelled    = errors.New("user context cancelled")
)

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

	if pqErr := pq.As(err, pqerror.QueryCanceled); pqErr != nil {
		return fmt.Errorf("%w: %w", ErrQueryTimeout, pqErr)
	}

	return err
}

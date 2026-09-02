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

	// Check for PostgreSQL query cancellation first — this is the most
	// specific indicator that a long-running query was the root cause.
	// When an upstream proxy disconnects before the query timeout fires,
	// the parent context is cancelled and PostgreSQL returns error 57014.
	// Checking the pq error before ctx.Err() ensures this is classified
	// as ErrQueryTimeout (→ 504) rather than ErrContextCancelled (→ 500).
	if pqErr := pq.As(err, pqerror.QueryCanceled); pqErr != nil {
		return fmt.Errorf("%w: %w", ErrQueryTimeout, pqErr)
	}

	// Check context error for non-pq cancellations
	if ctxErr := ctx.Err(); ctxErr == context.Canceled {
		return fmt.Errorf("%w: %w", ErrContextCancelled, ctxErr)
	} else if ctxErr == context.DeadlineExceeded {
		return fmt.Errorf("%w: %w", ErrContextQueryTimeout, ctxErr)
	}

	// Check if context deadline was exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrDatabaseTimeout, err)
	}

	return err
}

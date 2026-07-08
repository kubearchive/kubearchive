// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestWrapQueryError(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		err      error
		expected error
	}{
		{
			name:     "nil error returns nil",
			ctx:      context.Background(),
			err:      nil,
			expected: nil,
		},
		{
			name:     "context deadline exceeded",
			ctx:      context.Background(),
			err:      context.DeadlineExceeded,
			expected: ErrQueryTimeout,
		},
		{
			name: "context deadline exceeded from context",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(2 * time.Millisecond)
				return ctx
			}(),
			err:      errors.New("some database error"),
			expected: ErrQueryTimeout,
		},
		{
			name:     "postgresql error 57014 query_canceled",
			ctx:      context.Background(),
			err:      &pq.Error{Code: "57014", Message: "canceling statement due to user request"},
			expected: ErrQueryTimeout,
		},
		{
			name:     "other postgresql error not wrapped",
			ctx:      context.Background(),
			err:      &pq.Error{Code: "42P01", Message: "relation does not exist"},
			expected: &pq.Error{Code: "42P01", Message: "relation does not exist"},
		},
		{
			name:     "generic error not wrapped",
			ctx:      context.Background(),
			err:      errors.New("some other error"),
			expected: errors.New("some other error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapQueryError(tt.ctx, tt.err)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if tt.expected == ErrQueryTimeout {
				if !errors.Is(result, ErrQueryTimeout) {
					t.Errorf("expected ErrQueryTimeout, got %v", result)
				}
				return
			}

			// For specific error types, check the message matches
			if result.Error() != tt.expected.Error() {
				t.Errorf("expected error %q, got %q", tt.expected.Error(), result.Error())
			}
		})
	}
}

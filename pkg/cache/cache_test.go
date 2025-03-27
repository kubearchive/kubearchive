// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0
package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheOperations(t *testing.T) {
	testCases := []struct {
		name           string
		expireDuration time.Duration
		expected       any
		errorString    string
	}{
		{
			name:           "value is set and retrieved",
			expireDuration: 10 * time.Minute,
			expected:       "b",
			errorString:    "Expected value to be 'b'",
		},
		{
			name:           "value is expired",
			expireDuration: -10 * time.Minute, // make it expire instantly
			expected:       nil,
			errorString:    "Expected value to be 'nil' as it is expired.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := New()
			cache.Set("a", tc.expected, tc.expireDuration)
			value := cache.Get("a")

			assert.Equal(t, tc.expected, value, tc.errorString)
		})
	}
}

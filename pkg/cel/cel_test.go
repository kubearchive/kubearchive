// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cel

import (
	"context"
	"fmt"
	"testing"

	//"github.com/google/cel-go/common/types"
	"github.com/stretchr/testify/assert"
)

func TestNowFunction(t *testing.T) {
	testCases := []struct {
		name     string
		expr     string
		expected bool
		object   map[string]interface{}
	}{
		{
			name:     "earlier",
			expr:     "now() > timestamp(status.startTime)",
			expected: true,
			object:   map[string]interface{}{"status": map[string]any{"startTime": "2025-03-27T22:44:31Z"}},
		},
		{
			name:     "later",
			expr:     "now() < timestamp('2100-01-01T10:00:00Z')",
			expected: true,
			object:   map[string]interface{}{"status": map[string]any{"startTime": "2100-03-27T22:44:31Z"}},
		},
		{
			name:     "not earlier",
			expr:     "now() < timestamp(status.startTime)",
			expected: false,
			object:   map[string]interface{}{"status": map[string]any{"startTime": "2025-03-27T22:44:31Z"}},
		},
		{
			name:     "not later",
			expr:     "now() > timestamp('2100-01-01T10:00:00Z')",
			expected: false,
			object:   map[string]interface{}{"status": map[string]any{"startTime": "2100-03-27T22:44:31Z"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			program, e := CompileCELExpr(tc.expr)
			if e != nil {
				fmt.Println("Error in compilation: ", e)
			}
			result, _, _ := (*program).ContextEval(context.Background(), tc.object)
			assert.Equal(t, tc.expected, result.Value(), "Does not match")
		})
	}
}

// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilesInDir(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected map[string]string
		err      bool
	}{
		{
			name:     "error when directory does not exist",
			path:     "testdata/invalidPath",
			expected: nil,
			err:      true,
		},
		{
			name: "dir with files",
			path: "testdata/files",
			expected: map[string]string{
				"one":   "testdata/files/one",
				"two":   "testdata/files/two",
				"three": "testdata/files/three",
			},
			err: false,
		},
		{
			name: "dir with dir",
			path: "testdata/filesWithDir",
			expected: map[string]string{
				"one":   "testdata/filesWithDir/one",
				"two":   "testdata/filesWithDir/two",
				"three": "testdata/filesWithDir/three",
			},
			err: false,
		},
		{
			name: "dir with files that start with ..",
			path: "testdata/filesWithDotDot",
			expected: map[string]string{
				"one":   "testdata/filesWithDotDot/one",
				"two":   "testdata/filesWithDotDot/two",
				"three": "testdata/filesWithDotDot/three",
			},
			err: false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files, err := FilesInDir(tt.path)
			if tt.err {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, files)
		})
	}
}

func TestLoggingConfigFromFiles(t *testing.T) {
	testCases := []struct {
		name     string
		files    map[string]string
		expected map[string]string
		err      bool
	}{
		{
			name: "error when directory does not exist",
			files: map[string]string{
				"one":   "testdata/invalidPath/one",
				"two":   "testdata/invalidPath/two",
				"three": "testdata/invalidPath/three",
			},
			expected: nil,
			err:      true,
		},
		{
			name: "dir with files",
			files: map[string]string{
				"one":   "testdata/files/one",
				"two":   "testdata/files/two",
				"three": "testdata/files/three",
			},
			expected: map[string]string{
				"one":   "one",
				"two":   "two",
				"three": "three",
			},
			err: false,
		},
		{
			name: "dir with dir",
			files: map[string]string{
				"one":   "testdata/filesWithDir/one",
				"two":   "testdata/filesWithDir/two",
				"three": "testdata/filesWithDir/three",
			},
			expected: map[string]string{
				"one":   "one",
				"two":   "two",
				"three": "three",
			},
			err: false,
		},
		{
			name: "dir with files that start with ..",
			files: map[string]string{
				"one":   "testdata/filesWithDotDot/one",
				"two":   "testdata/filesWithDotDot/two",
				"three": "testdata/filesWithDotDot/three",
			},
			expected: map[string]string{
				"one":   "one",
				"two":   "two",
				"three": "three",
			},
			err: false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files, err := LoggingConfigFromFiles(tt.files)
			if tt.err {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tt.expected, files)
		})
	}
}

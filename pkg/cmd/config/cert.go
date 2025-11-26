// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
)

func LoadCertData(certPath string) (string, []byte, error) {

	if certPath == "" {
		return "", nil, fmt.Errorf("cert path is empty")
	}

	// Expand path if it starts with ~
	expandedPath := certPath
	if expandedPath[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		expandedPath = homeDir + expandedPath[1:]
	}

	certData, err := os.ReadFile(expandedPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read certificate file %s: %w", expandedPath, err)
	}

	return expandedPath, certData, nil
}

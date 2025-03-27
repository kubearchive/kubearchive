// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"
	"os"
	"strings"
)

const LoggingDirEnvVar = "KRONICLER_LOGGING_DIR"

func FilesInDir(path string) (map[string]string, error) {
	dir, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string)
	for _, file := range dir {
		// kubernetes will create symlinks that start with '..' that should be ignored
		if !file.IsDir() && !strings.HasPrefix(file.Name(), "..") {
			var filePath strings.Builder
			fmt.Fprintf(&filePath, "%s/%s", path, file.Name())
			files[file.Name()] = filePath.String()
		}
	}
	return files, nil
}

func LoggingConfigFromFiles(files map[string]string) (map[string]string, error) {
	conf := make(map[string]string)
	for name, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		} else {
			conf[name] = strings.TrimSpace(string(data))
		}
	}
	return conf, nil
}

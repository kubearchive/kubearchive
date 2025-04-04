// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/files"
	"github.com/kubearchive/kubearchive/pkg/logurls"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	celPrefix        = "cel:"
	containerNameCel = "cel:spec.containers.map(m, m.name)"
	jsonPathKey      = "LOG_URL_JSONPATH"
)

func getKubeArchiveLoggingConfig() (map[string]string, error) {
	loggingDir, exists := os.LookupEnv(files.LoggingDirEnvVar)
	if !exists {
		return nil, errors.New("environment variable not set: " + files.LoggingDirEnvVar)
	}
	configFiles, err := files.FilesInDir(loggingDir)
	if err != nil {
		return nil, fmt.Errorf("could not read logging config: %w", err)
	}
	if len(configFiles) == 0 {
		return nil, errors.New("no logging configuration specified")
	}

	loggingConf, err := files.LoggingConfigFromFiles(configFiles)
	if err != nil {
		return nil, fmt.Errorf("could not get value for logging config: %w", err)
	}
	return loggingConf, nil
}

type UrlBuilder struct {
	jsonPath string
	logMap   map[string]interface{}
}

func NewUrlBuilder() (*UrlBuilder, error) {
	loggingConf, err := getKubeArchiveLoggingConfig()
	if err != nil {
		return nil, err
	}
	_, exists := loggingConf[logurls.LogURL]
	if !exists {
		return nil, errors.New("invalid logging config. The kubearchive-logging ConfigMap must have a key 'LOG_URL'")
	}
	// Set CONTAINER_NAME and overwrite it if already defined
	loggingConf[logurls.ContainerName] = containerNameCel
	logMap := make(map[string]interface{})
	for key, val := range loggingConf {
		celExpr, isCelExpr := strings.CutPrefix(val, celPrefix)
		if isCelExpr {
			celProg, err := ocel.CompileCELExpr(celExpr)
			if err != nil {
				return nil, fmt.Errorf(
					"cannot create UrlBuilder. CEL expression '%s' does not compile: %w",
					celExpr,
					err,
				)
			}
			logMap[key] = celProg
		} else {
			logMap[key] = val
		}
	}
	return &UrlBuilder{jsonPath: loggingConf[jsonPathKey], logMap: logMap}, nil
}

func (ub *UrlBuilder) Urls(ctx context.Context, data *unstructured.Unstructured) ([]models.LogTuple, error) {
	return logurls.GenerateLogURLs(ctx, ub.logMap, data)
}

func (ub *UrlBuilder) GetJsonPath() string {
	return ub.jsonPath
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubearchive/kubearchive/cmd/sink/k8s"
	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/log_urls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
)

const (
	configCmName     = "kubearchive-config"
	loggingCmKeyName = "logging-configmap-name"
	logUrlKey        = "LOG_URL"
	celPrefix        = "cel:"
)

func getKubeArchiveLoggingCm(ctx context.Context, kubeClient kubernetes.Interface) (*corev1.ConfigMap, error) {
	kaConfigCm, err := kubeClient.CoreV1().ConfigMaps(k8s.KubeArchiveNamespace).Get(
		ctx,
		configCmName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("Could not get ConfigMap: %w", err)
	}
	loggingCmName, exists := kaConfigCm.Data[loggingCmKeyName]
	if !exists {
		return nil, fmt.Errorf("No logging ConfigMap was provided")
	}
	loggingCm, err := kubeClient.CoreV1().ConfigMaps(k8s.KubeArchiveNamespace).Get(
		ctx,
		loggingCmName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("Could not get ConfigMap: %w", err)
	}
	return loggingCm, nil
}

type UrlBuilder struct {
	logMap map[string]interface{}
}

func NewUrlBuilder(ctx context.Context, kubeClient kubernetes.Interface) (*UrlBuilder, error) {
	if kubeClient == nil {
		return &UrlBuilder{}, fmt.Errorf("Cannot create UrlBuilder with nil kubernetes client")
	}
	loggingCm, err := getKubeArchiveLoggingCm(ctx, kubeClient)
	if err != nil {
		return &UrlBuilder{}, fmt.Errorf("Cannot create UrlBuilder. Could not get logging ConfigMap: %w", err)
	}
	logMap := make(map[string]interface{})
	for key, val := range loggingCm.Data {
		celExpr, isCelExpr := strings.CutPrefix(val, celPrefix)
		if isCelExpr {
			celProg, err := ocel.CompileCELExpr(celExpr)
			if err != nil {
				return &UrlBuilder{}, fmt.Errorf("Cannot create UrlBuilder. CEL expression does not compile: %w", err)
			}
			logMap[key] = celProg
		} else {
			logMap[key] = val
		}
	}
	return &UrlBuilder{logMap: logMap}, nil
}

func (ub *UrlBuilder) Urls(ctx context.Context, data *unstructured.Unstructured) ([]string, error) {
	return log_urls.GenerateLogURLs(ctx, ub.logMap, data)
}

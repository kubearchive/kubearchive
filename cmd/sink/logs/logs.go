// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/log_urls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
)

const celPrefix = "cel:"

var (
	configCmRef = corev1.ObjectReference{
		Kind:      "ConfigMap",
		Name:      "kubearchive-config",
		Namespace: "kubearchive",
	}
	loggingCmRef = corev1.ObjectReference{
		Kind:      "ConfigMap",
		Namespace: "kubearchive",
	}
	loggingCmSelector = corev1.ConfigMapKeySelector{
		Key: "logging-configmap-name",
	}
)

func getKubeArchiveLoggingCm(ctx context.Context, kubeClient kubernetes.Interface) *corev1.ConfigMap {
	emptyCm := &corev1.ConfigMap{Data: map[string]string{}}
	kaConfigCm, err := kubeClient.CoreV1().ConfigMaps(configCmRef.Namespace).Get(
		ctx,
		configCmRef.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		slog.Warn(
			"error while getting ConfigMap",
			"ConfigMap",
			fmt.Sprintf("%s/%s", configCmRef.Namespace, configCmRef.Name),
			"error",
			err.Error(),
		)
		return emptyCm
	}
	var exists bool
	loggingCmRef.Name, exists = kaConfigCm.Data[loggingCmSelector.Key]
	if !exists {
		slog.Warn("logging ConfigMap not provided", "key", loggingCmSelector.Key)
		return emptyCm
	}
	loggingCm, err := kubeClient.CoreV1().ConfigMaps(loggingCmRef.Namespace).Get(
		ctx,
		loggingCmRef.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		slog.Warn(
			"error while getting ConfigMap",
			"ConfigMap",
			fmt.Sprintf("%s/%s", loggingCmRef.Namespace, loggingCmRef.Name),
			"error",
			err.Error(),
		)
		return emptyCm
	}
	return loggingCm
}

type UrlBuilder struct {
	logMap map[string]interface{}
}

func NewUrlBuilder(ctx context.Context, kubeClient kubernetes.Interface) (*UrlBuilder, error) {
	if kubeClient == nil {
		return &UrlBuilder{}, fmt.Errorf("Cannot create UrlBuilder with nil kubernetes client")
	}
	loggingCm := getKubeArchiveLoggingCm(ctx, kubeClient)
	logMap := make(map[string]interface{})
	for key, val := range loggingCm.Data {
		celExpr, isCelExpr := strings.CutPrefix(val, celPrefix)
		if isCelExpr {
			celProg, err := ocel.CompileCELExpr(celExpr)
			if err != nil {
				return &UrlBuilder{}, fmt.Errorf(
					"Cannot create UrlBuilder. CEL expression '%s' does not compile: %w",
					celExpr,
					err,
				)
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

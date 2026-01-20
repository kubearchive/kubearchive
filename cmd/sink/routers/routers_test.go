// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/cmd/sink/logs"
	fakeDb "github.com/kubearchive/kubearchive/pkg/database/fake"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/files"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
)

func setupRouter(
	t testing.TB,
	db interfaces.DBWriter,
	k8sClient dynamic.Interface,
	builder *logs.UrlBuilder,
) *gin.Engine {
	t.Helper()
	router := gin.Default()
	ctrl := NewController(db, k8sClient, builder)
	router.POST("/", ctrl.ReceiveCloudEvent)
	router.GET("/livez", ctrl.Livez)
	router.GET("/readyz", ctrl.Readyz)
	return router
}

func setupClient(t testing.TB, objects ...runtime.Object) *fake.FakeDynamicClient {
	t.Helper()
	testScheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = batchv1.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = corev1.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = kubearchiveapi.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	return fake.NewSimpleDynamicClient(testScheme, objects...)
}

func TestReceiveCloudEvents(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		httpStatus int
		records    int
		logs       int
	}{
		{
			name:       "Valid CloudEvent with kubernetes resource",
			file:       "testdata/CE-job.json",
			httpStatus: http.StatusAccepted,
			records:    1,
		},
		{
			name:       "Request body is not a CloudEvent",
			file:       "testdata/not-CE.json",
			httpStatus: http.StatusBadRequest,
			records:    0,
		},
		{
			name:       "Valid CloudEvent but data is not kubernetes resource",
			file:       "testdata/CE-not-k8s.json",
			httpStatus: http.StatusUnprocessableEntity,
			records:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := fakeDb.NewFakeDatabase([]*unstructured.Unstructured{}, []fakeDb.LogUrlRow{}, "$.")
			builder, _ := logs.NewUrlBuilder()
			router := setupRouter(t, db, nil, builder)
			res := httptest.NewRecorder()
			reader, err := os.Open(tt.file)
			if err != nil {
				assert.FailNow(t, err.Error())
			}
			t.Cleanup(func() { reader.Close() })
			req := httptest.NewRequest(http.MethodPost, "/", reader)
			req.Header.Add("Content-Type", "application/cloudevents+json")
			router.ServeHTTP(res, req)

			assert.Equal(t, tt.httpStatus, res.Code)
			assert.Equal(t, tt.records, db.NumResources())
			assert.Equal(t, 0, db.NumLogUrls())
		})
	}
}

func TestResourceWriteFails(t *testing.T) {
	t.Setenv(files.LoggingDirEnvVar, "testdata/loggingconfig")

	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Job",
			"apiVersion": "batch/v1",
			"metadata": map[string]interface{}{
				"name":      "generate-log-1-28968184",
				"namespace": "generate-logs-cronjobs",
			},
		},
	}
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Pod",
			"apiVersion": "v1",
			"metadata": map[string]interface{}{
				"name":      "generate-log-1-28806900-sp286",
				"namespace": "generate-logs-cronjobs",
			},
		},
	}
	objs := []runtime.Object{job, pod}

	tests := []struct {
		name            string
		files           []string
		httpStatus      int
		archive         []string
		delete          []string
		archiveOnDelete []string
		clusterObjs     []runtime.Object
	}{
		{
			name:            "Archive Jobs from CloudEvents",
			files:           []string{"testdata/CE-job.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{"Job"},
			delete:          []string{},
			archiveOnDelete: []string{},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "Archive Pod from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-1-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{"Pod"},
			delete:          []string{},
			archiveOnDelete: []string{},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "Archive Pod with 3 containers from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-3-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{"Pod"},
			delete:          []string{},
			archiveOnDelete: []string{},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "ArchiveOnDelete Jobs from CloudEvents",
			files:           []string{"testdata/CE-job-delete.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{},
			archiveOnDelete: []string{"Job"},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "ArchiveOnDelete Pod from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-1-container-delete.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{},
			archiveOnDelete: []string{"Pod"},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "ArchiveOnDelete Pod with 3 containers from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-3-container-delete.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{},
			archiveOnDelete: []string{"Pod"},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "Delete Job from CloudEvent",
			files:           []string{"testdata/CE-job.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Job"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
		{
			name:            "Delete Pod from CloudEvent with log urls",
			files:           []string{"testdata/CE-pod-1-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Pod"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
		{
			name:            "Delete Pod with 3 containers from CloudEvent with log urls",
			files:           []string{"testdata/CE-pod-3-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Pod"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
		{
			name:            "Delete Pod that does not exist",
			files:           []string{"testdata/CE-pod-does-not-exist.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Pod"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
	}

	testScheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = batchv1.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = corev1.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleDynamicClient(testScheme, tt.clusterObjs...)
			db := fakeDb.NewFakeDatabaseWithError(errors.New("test error"))
			builder, err := logs.NewUrlBuilder()
			if err != nil {
				assert.FailNow(t, err.Error())
			}
			router := setupRouter(t, db, client, builder)
			for _, file := range tt.files {
				res := httptest.NewRecorder()
				reader, err := os.Open(file)
				if err != nil {
					assert.FailNow(t, err.Error())
				}
				t.Cleanup(func() { reader.Close() })
				req := httptest.NewRequest(http.MethodPost, "/", reader)
				req.Header.Add("Content-Type", "application/cloudevents+json")
				router.ServeHTTP(res, req)

				assert.Equal(t, tt.httpStatus, res.Code)
			}
		})
	}
}

func TestLogUrlWriteFails(t *testing.T) {
	t.Setenv(files.LoggingDirEnvVar, "testdata/loggingconfig")

	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Job",
			"apiVersion": "batch/v1",
			"metadata": map[string]interface{}{
				"name":      "generate-log-1-28968184",
				"namespace": "generate-logs-cronjobs",
			},
		},
	}
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Pod",
			"apiVersion": "v1",
			"metadata": map[string]interface{}{
				"name":      "generate-log-1-28806900-sp286",
				"namespace": "generate-logs-cronjobs",
			},
		},
	}
	objs := []runtime.Object{job, pod}

	tests := []struct {
		name            string
		files           []string
		httpStatus      int
		archive         []string
		delete          []string
		archiveOnDelete []string
		clusterObjs     []runtime.Object
	}{
		{
			name:            "Archive Jobs from CloudEvents",
			files:           []string{"testdata/CE-job.json"},
			httpStatus:      http.StatusAccepted,
			archive:         []string{"Job"},
			delete:          []string{},
			archiveOnDelete: []string{},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "Archive Pod from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-1-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{"Pod"},
			delete:          []string{},
			archiveOnDelete: []string{},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "Archive Pod with 3 containers from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-3-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{"Pod"},
			delete:          []string{},
			archiveOnDelete: []string{},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "ArchiveOnDelete Jobs from CloudEvents",
			files:           []string{"testdata/CE-job-delete.json"},
			httpStatus:      http.StatusAccepted,
			archive:         []string{},
			delete:          []string{},
			archiveOnDelete: []string{"Job"},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "ArchiveOnDelete Pod from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-1-container-delete.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{},
			archiveOnDelete: []string{"Pod"},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "ArchiveOnDelete Pod with 3 containers from CloudEvents with log urls",
			files:           []string{"testdata/CE-pod-3-container-delete.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{},
			archiveOnDelete: []string{"Pod"},
			clusterObjs:     []runtime.Object{},
		},
		{
			name:            "Delete Job from CloudEvent",
			files:           []string{"testdata/CE-job.json"},
			httpStatus:      http.StatusAccepted,
			archive:         []string{},
			delete:          []string{"Job"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
		{
			name:            "Delete Pod from CloudEvent with log urls",
			files:           []string{"testdata/CE-pod-1-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Pod"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
		{
			name:            "Delete Pod with 3 containers from CloudEvent with log urls",
			files:           []string{"testdata/CE-pod-3-container.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Pod"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
		{
			name:            "Delete Pod that does not exist",
			files:           []string{"testdata/CE-pod-does-not-exist.json"},
			httpStatus:      http.StatusInternalServerError,
			archive:         []string{},
			delete:          []string{"Pod"},
			archiveOnDelete: []string{},
			clusterObjs:     objs,
		},
	}

	testScheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = batchv1.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	err = corev1.AddToScheme(testScheme)
	if err != nil {
		assert.FailNow(t, err.Error())
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleDynamicClient(testScheme, tt.clusterObjs...)
			db := fakeDb.NewFakeDatabaseWithUrlError(errors.New("test error"))
			builder, err := logs.NewUrlBuilder()
			if err != nil {
				assert.FailNow(t, err.Error())
			}
			router := setupRouter(t, db, client, builder)
			for _, file := range tt.files {
				res := httptest.NewRecorder()
				reader, err := os.Open(file)
				if err != nil {
					assert.FailNow(t, err.Error())
				}
				t.Cleanup(func() { reader.Close() })
				req := httptest.NewRequest(http.MethodPost, "/", reader)
				req.Header.Add("Content-Type", "application/cloudevents+json")
				router.ServeHTTP(res, req)

				assert.Equal(t, tt.httpStatus, res.Code)
			}
		})
	}
}

func TestLivez(t *testing.T) {
	db := fakeDb.NewFakeDatabase([]*unstructured.Unstructured{}, []fakeDb.LogUrlRow{}, "$.")
	builder, _ := logs.NewUrlBuilder()
	router := setupRouter(t, db, nil, builder)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	router.ServeHTTP(res, req)

	expected, _ := json.Marshal(gin.H{
		"Code":          http.StatusOK,
		"ginMode":       "debug",
		"openTelemetry": "disabled",
		"message":       "healthy",
	})
	assert.Equal(t, res.Body.Bytes(), expected)
	assert.Equal(t, res.Code, http.StatusOK)
}

func TestReadyz(t *testing.T) {
	testCases := []struct {
		name         string
		dbConnReady  bool
		namespaceSet bool
		k8sApiConn   bool
		expected     int
		clusterObjs  []runtime.Object
	}{
		{
			name:        "Sink is ready",
			dbConnReady: true,
			k8sApiConn:  true,
			expected:    http.StatusOK,
			clusterObjs: []runtime.Object{},
		},
		{
			name:        "Database is not ready",
			dbConnReady: false,
			k8sApiConn:  true,
			expected:    http.StatusServiceUnavailable,
			clusterObjs: []runtime.Object{},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			var db interfaces.DBWriter
			builder, _ := logs.NewUrlBuilder()
			if tt.dbConnReady {
				db = fakeDb.NewFakeDatabase([]*unstructured.Unstructured{}, []fakeDb.LogUrlRow{}, "$.")
			} else {
				db = fakeDb.NewFakeDatabaseWithError(errors.New("test error"))
			}
			client := setupClient(t, tt.clusterObjs...)
			router := setupRouter(t, db, client, builder)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			router.ServeHTTP(res, req)

			assert.Equal(t, tt.expected, res.Code)
		})
	}
}

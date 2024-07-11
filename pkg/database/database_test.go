package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func NewMock() (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		log.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	return db, mock
}

func TestQueryResources(t *testing.T) {
	kind := "CronJob"
	apiVersion := "batch/v1"
	group := "batch"
	version := "v1"
	db, mock := NewMock()
	database := Database{db: db, resourceTableName: "test"}

	query := "SELECT data FROM test WHERE kind=\\$1 AND api_version=\\$2"

	rows := sqlmock.NewRows([]string{"data"}).
		AddRow(json.RawMessage(`{"kind": "CronJob", "spec": {"suspend": false, "schedule": "*/30 * * * *", "jobTemplate": {"spec": {"template": {"spec": {"dnsPolicy": "ClusterFirst", "containers": [{"env": [{"name": "GITLAB_TOKEN", "valueFrom": {"secretKeyRef": {"key": "secrettext", "name": "gitlab-api-token"}}}], "name": "clean-widget", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master", "command": ["/home/jenkins/bin/clean-ci-projects.sh", "--interval=2 hours", "--startswith=cpaas-ci-widget-"], "resources": {}, "imagePullPolicy": "IfNotPresent", "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}, {"env": [{"name": "GITLAB_TOKEN", "valueFrom": {"secretKeyRef": {"key": "secrettext", "name": "gitlab-api-token"}}}], "name": "clean-test-product-ci", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master", "command": ["/home/jenkins/bin/clean-ci-projects.sh", "--interval=2 hours", "--startswith=cpaas-ci-test-product-ci-"], "resources": {}, "imagePullPolicy": "IfNotPresent", "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}], "restartPolicy": "OnFailure", "schedulerName": "default-scheduler", "serviceAccount": "jenkins-cpaas-ci", "securityContext": {}, "serviceAccountName": "jenkins-cpaas-ci", "terminationGracePeriodSeconds": 30}, "metadata": {"name": "clean-ci-projects", "creationTimestamp": null}}, "completions": 1, "parallelism": 1, "backoffLimit": 6, "activeDeadlineSeconds": 1800}, "metadata": {"creationTimestamp": null}}, "concurrencyPolicy": "Forbid", "failedJobsHistoryLimit": 3, "startingDeadlineSeconds": 200, "successfulJobsHistoryLimit": 12}, "status": {"lastScheduleTime": "2024-05-26T21:00:00Z", "lastSuccessfulTime": "2024-05-26T21:00:09Z"}, "metadata": {"uid": "108e947d-ab1c-4331-ab39-5d203fc17115", "name": "clean-ci-projects", "namespace": "cpaas-ci", "generation": 1, "annotations": {"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"batch/v1\",\"kind\":\"CronJob\",\"metadata\":{\"annotations\":{},\"name\":\"clean-ci-projects\",\"namespace\":\"cpaas-ci\"},\"spec\":{\"concurrencyPolicy\":\"Forbid\",\"failedJobsHistoryLimit\":3,\"jobTemplate\":{\"spec\":{\"activeDeadlineSeconds\":1800,\"backoffLimit\":6,\"completions\":1,\"parallelism\":1,\"template\":{\"metadata\":{\"name\":\"clean-ci-projects\"},\"spec\":{\"containers\":[{\"command\":[\"/home/jenkins/bin/clean-ci-projects.sh\",\"--interval=2 hours\",\"--startswith=cpaas-ci-widget-\"],\"env\":[{\"name\":\"GITLAB_TOKEN\",\"valueFrom\":{\"secretKeyRef\":{\"key\":\"secrettext\",\"name\":\"gitlab-api-token\"}}}],\"image\":\"image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master\",\"name\":\"clean-widget\"},{\"command\":[\"/home/jenkins/bin/clean-ci-projects.sh\",\"--interval=2 hours\",\"--startswith=cpaas-ci-test-product-ci-\"],\"env\":[{\"name\":\"GITLAB_TOKEN\",\"valueFrom\":{\"secretKeyRef\":{\"key\":\"secrettext\",\"name\":\"gitlab-api-token\"}}}],\"image\":\"image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master\",\"name\":\"clean-test-product-ci\"}],\"restartPolicy\":\"OnFailure\",\"serviceAccountName\":\"jenkins-cpaas-ci\"}}}},\"schedule\":\"*/30 * * * *\",\"startingDeadlineSeconds\":200,\"successfulJobsHistoryLimit\":12,\"suspend\":false}}\n"}, "resourceVersion": "1980468989", "creationTimestamp": "2024-04-05T09:58:03Z"}, "apiVersion": "batch/v1"}`))

	mock.ExpectQuery(query).WithArgs(kind, apiVersion).WillReturnRows(rows)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	resources, err := database.QueryResources(ctx, kind, group, version)
	assert.NotNil(t, resources)
	assert.NotEqual(t, 0, len(resources))
	assert.NoError(t, err)
}

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

const (
	kind                     = "CronJob"
	cronJobApiVersion        = "batch/v1"
	podApiVersion            = "v1"
	group                    = "batch"
	version                  = "v1"
	namespace                = "cpaas-ci"
	queryResources           = "SELECT data FROM resource WHERE kind=(\\$1|\\?) AND api_version=(\\$2|\\?)"
	queryNamespacedResources = "SELECT data FROM resource WHERE kind=(\\$1|\\?) AND api_version=(\\$2|\\?) AND namespace=(\\$3|\\?)"
	testCronJobResource      = `{"kind": "CronJob", "spec": {"suspend": false, "schedule": "*/30 * * * *", "jobTemplate": {"spec": {"template": {"spec": {"dnsPolicy": "ClusterFirst", "containers": [{"env": [{"name": "GITLAB_TOKEN", "valueFrom": {"secretKeyRef": {"key": "secrettext", "name": "gitlab-api-token"}}}], "name": "clean-widget", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master", "command": ["/home/jenkins/bin/clean-ci-projects.sh", "--interval=2 hours", "--startswith=cpaas-ci-widget-"], "resources": {}, "imagePullPolicy": "IfNotPresent", "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}, {"env": [{"name": "GITLAB_TOKEN", "valueFrom": {"secretKeyRef": {"key": "secrettext", "name": "gitlab-api-token"}}}], "name": "clean-test-product-ci", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master", "command": ["/home/jenkins/bin/clean-ci-projects.sh", "--interval=2 hours", "--startswith=cpaas-ci-test-product-ci-"], "resources": {}, "imagePullPolicy": "IfNotPresent", "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}], "restartPolicy": "OnFailure", "schedulerName": "default-scheduler", "serviceAccount": "jenkins-cpaas-ci", "securityContext": {}, "serviceAccountName": "jenkins-cpaas-ci", "terminationGracePeriodSeconds": 30}, "metadata": {"name": "clean-ci-projects", "creationTimestamp": null}}, "completions": 1, "parallelism": 1, "backoffLimit": 6, "activeDeadlineSeconds": 1800}, "metadata": {"creationTimestamp": null}}, "concurrencyPolicy": "Forbid", "failedJobsHistoryLimit": 3, "startingDeadlineSeconds": 200, "successfulJobsHistoryLimit": 12}, "status": {"lastScheduleTime": "2024-05-26T21:00:00Z", "lastSuccessfulTime": "2024-05-26T21:00:09Z"}, "metadata": {"uid": "108e947d-ab1c-4331-ab39-5d203fc17115", "name": "clean-ci-projects", "namespace": "cpaas-ci", "generation": 1, "annotations": {"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"batch/v1\",\"kind\":\"CronJob\",\"metadata\":{\"annotations\":{},\"name\":\"clean-ci-projects\",\"namespace\":\"cpaas-ci\"},\"spec\":{\"concurrencyPolicy\":\"Forbid\",\"failedJobsHistoryLimit\":3,\"jobTemplate\":{\"spec\":{\"activeDeadlineSeconds\":1800,\"backoffLimit\":6,\"completions\":1,\"parallelism\":1,\"template\":{\"metadata\":{\"name\":\"clean-ci-projects\"},\"spec\":{\"containers\":[{\"command\":[\"/home/jenkins/bin/clean-ci-projects.sh\",\"--interval=2 hours\",\"--startswith=cpaas-ci-widget-\"],\"env\":[{\"name\":\"GITLAB_TOKEN\",\"valueFrom\":{\"secretKeyRef\":{\"key\":\"secrettext\",\"name\":\"gitlab-api-token\"}}}],\"image\":\"image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master\",\"name\":\"clean-widget\"},{\"command\":[\"/home/jenkins/bin/clean-ci-projects.sh\",\"--interval=2 hours\",\"--startswith=cpaas-ci-test-product-ci-\"],\"env\":[{\"name\":\"GITLAB_TOKEN\",\"valueFrom\":{\"secretKeyRef\":{\"key\":\"secrettext\",\"name\":\"gitlab-api-token\"}}}],\"image\":\"image-registry.openshift-image-registry.svc:5000/cpaas-ci/cpaas-slave-ocp4:master\",\"name\":\"clean-test-product-ci\"}],\"restartPolicy\":\"OnFailure\",\"serviceAccountName\":\"jenkins-cpaas-ci\"}}}},\"schedule\":\"*/30 * * * *\",\"startingDeadlineSeconds\":200,\"successfulJobsHistoryLimit\":12,\"suspend\":false}}\n"}, "resourceVersion": "1980468989", "creationTimestamp": "2024-04-05T09:58:03Z"}, "apiVersion": "batch/v1"}`
	testPodResource          = `{"kind": "Pod", "apiVersion": "v1", "spec": {"volumes": [{"name": "otel-config", "configMap": {"name": "otel-collector-config", "items": [{"key": "otelcol.yaml", "path": "otelcol.yaml"}], "optional": true, "defaultMode": 420}}, {"name": "kube-api-access-njsk9", "projected": {"sources": [{"serviceAccountToken": {"path": "token", "expirationSeconds": 3607}}, {"configMap": {"name": "kube-root-ca.crt", "items": [{"key": "ca.crt", "path": "ca.crt"}]}}, {"downwardAPI": {"items": [{"path": "namespace", "fieldRef": {"fieldPath": "metadata.namespace", "apiVersion": "v1"}}]}}, {"configMap": {"name": "openshift-service-ca.crt", "items": [{"key": "service-ca.crt", "path": "service-ca.crt"}]}}], "defaultMode": 420}}], "nodeName": "ip-10-30-218-170.ec2.internal", "priority": 0, "dnsPolicy": "ClusterFirst", "containers": [{"args": ["--config=/etc/otel/otelcol.yaml"], "name": "otel-collector", "image": "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib@sha256:1720f9ce46441e0bb6e4b9ac448c476a950db0767fe774bb73877ecd46017dd7", "ports": [{"protocol": "TCP", "containerPort": 4317}, {"protocol": "TCP", "containerPort": 8889}], "resources": {"limits": {"cpu": "1", "memory": "2Gi"}, "requests": {"cpu": "200m", "memory": "100Mi"}}, "volumeMounts": [{"name": "otel-config", "subPath": "otelcol.yaml", "readOnly": true, "mountPath": "/etc/otel/otelcol.yaml"}, {"name": "kube-api-access-njsk9", "readOnly": true, "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"}], "livenessProbe": {"httpGet": {"path": "/", "port": 13133, "scheme": "HTTP"}, "periodSeconds": 10, "timeoutSeconds": 30, "failureThreshold": 30, "successThreshold": 1, "initialDelaySeconds": 1800}, "readinessProbe": {"httpGet": {"path": "/", "port": 13133, "scheme": "HTTP"}, "periodSeconds": 10, "timeoutSeconds": 30, "failureThreshold": 300, "successThreshold": 1, "initialDelaySeconds": 300}, "imagePullPolicy": "IfNotPresent", "securityContext": {"runAsUser": 1000930000, "capabilities": {"drop": ["ALL"]}, "runAsNonRoot": true, "allowPrivilegeEscalation": false}, "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}], "tolerations": [{"key": "node.kubernetes.io/not-ready", "effect": "NoExecute", "operator": "Exists", "tolerationSeconds": 300}, {"key": "node.kubernetes.io/unreachable", "effect": "NoExecute", "operator": "Exists", "tolerationSeconds": 300}, {"key": "node.kubernetes.io/memory-pressure", "effect": "NoSchedule", "operator": "Exists"}], "restartPolicy": "Always", "schedulerName": "default-scheduler", "serviceAccount": "default", "securityContext": {"fsGroup": 1000930000, "seLinuxOptions": {"level": "s0:c31,c0"}, "seccompProfile": {"type": "RuntimeDefault"}}, "imagePullSecrets": [{"name": "cpaas-container-registries"}, {"name": "default-dockercfg-rhb7z"}], "preemptionPolicy": "PreemptLowerPriority", "enableServiceLinks": true, "serviceAccountName": "default", "terminationGracePeriodSeconds": 30}, "status": {"phase": "Running", "podIP": "10.131.2.206", "hostIP": "10.30.218.170", "podIPs": [{"ip": "10.131.2.206"}], "qosClass": "Burstable", "startTime": "2024-04-05T09:57:32Z", "conditions": [{"type": "Initialized", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T09:57:32Z"}, {"type": "Ready", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T10:02:42Z"}, {"type": "ContainersReady", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T10:02:42Z"}, {"type": "PodScheduled", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T09:57:32Z"}], "containerStatuses": [{"name": "otel-collector", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci-widget-o6uljbey/opentelemetry-collector-contrib:0.64.1", "ready": true, "state": {"running": {"startedAt": "2024-04-05T09:57:34Z"}}, "imageID": "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib@sha256:1720f9ce46441e0bb6e4b9ac448c476a950db0767fe774bb73877ecd46017dd7", "started": true, "lastState": {}, "containerID": "cri-o://b6622cb6edcf8a9319771fd21c94d1796bc0d3a3f9b06c4cb44f154cadc0b06f", "restartCount": 0}]}, "metadata": {"uid": "42422d92-1a72-418d-97cf-97019c2d56e8", "name": "otelcollector-85fc74bc47-bqpbq", "labels": {"app": "otelcollector", "otel-infra": "otel-pod", "pod-template-hash": "85fc74bc47"}, "namespace": "cpaas-ci", "annotations": {"openshift.io/scc": "restricted-v2", "k8s.v1.cni.cncf.io/network-status": "[{\n    \"name\": \"openshift-sdn\",\n    \"interface\": \"eth0\",\n    \"ips\": [\n        \"10.131.2.206\"\n    ],\n    \"default\": true,\n    \"dns\": {}\n}]", "seccomp.security.alpha.kubernetes.io/pod": "runtime/default", "alpha.image.policy.openshift.io/resolve-names": "*"}, "generateName": "otelcollector-85fc74bc47-", "ownerReferences": [{"uid": "852e6139-ad94-44e1-a813-f70b7ab1c033", "kind": "ReplicaSet", "name": "otelcollector-85fc74bc47", "apiVersion": "apps/v1", "controller": true, "blockOwnerDeletion": true}], "resourceVersion": "1883964183", "creationTimestamp": "2024-04-05T09:57:32Z"} }`
)

var tests = []struct {
	name     string
	database *Database
}{
	{
		name:     "postgresql",
		database: &Database{info: &PostgreSQLDatabaseInfo{}},
	},
	{
		name:     "mariadb",
		database: &Database{info: &MariaDBDatabaseInfo{}},
	},
}

func NewMock() (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		log.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	return db, mock
}

func TestQueryResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = db

			rows := sqlmock.NewRows([]string{"data"}).
				AddRow(json.RawMessage(testCronJobResource))

			mock.ExpectQuery(queryResources).WithArgs(kind, cronJobApiVersion).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			resources, err := tt.database.QueryResources(ctx, kind, group, version)
			assert.NotNil(t, resources)
			assert.NotEqual(t, 0, len(resources))
			assert.NoError(t, err)
		})
	}
}

func TestQueryNamespacedResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = db

			rows := sqlmock.NewRows([]string{"data"}).
				AddRow(json.RawMessage(testCronJobResource))

			mock.ExpectQuery(queryNamespacedResources).WithArgs(kind, cronJobApiVersion, namespace).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			resources, err := tt.database.QueryNamespacedResources(ctx, kind, group, version, namespace)
			assert.NotNil(t, resources)
			assert.NotEqual(t, 0, len(resources))
			assert.NoError(t, err)
		})
	}
}

func TestCoreQueryResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = db

			rows := sqlmock.NewRows([]string{"data"}).
				AddRow(json.RawMessage(testPodResource))

			mock.ExpectQuery(queryResources).WithArgs(kind, podApiVersion).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			resources, err := tt.database.QueryCoreResources(ctx, kind, version)
			assert.NotNil(t, resources)
			assert.NotEqual(t, 0, len(resources))
			assert.NoError(t, err)
		})
	}
}

func TestQueryNamespacedCoreResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = db

			rows := sqlmock.NewRows([]string{"data"}).
				AddRow(json.RawMessage(testPodResource))

			mock.ExpectQuery(queryNamespacedResources).WithArgs(kind, podApiVersion, namespace).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			resources, err := tt.database.QueryNamespacedCoreResources(ctx, kind, version, namespace)
			assert.NotNil(t, resources)
			assert.NotEqual(t, 0, len(resources))
			assert.NoError(t, err)
		})
	}
}

func TestPing(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = db
			mock.ExpectPing()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			assert.Nil(t, tt.database.Ping(ctx))
		})
	}
}

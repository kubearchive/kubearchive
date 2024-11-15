package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

const (
	kind              = "CronJob"
	cronJobApiVersion = "batch/v1"
	podApiVersion     = "v1"
	cronJobName       = "test-cronjob"
	podName           = "test-pod"
	version           = "v1"
	namespace         = "cpaas-ci"
	testPodResource   = `{"kind": "Pod", "apiVersion": "v1", "spec": {"volumes": [{"name": "otel-config", "configMap": {"name": "otel-collector-config", "items": [{"key": "otelcol.yaml", "path": "otelcol.yaml"}], "optional": true, "defaultMode": 420}}, {"name": "kube-api-access-njsk9", "projected": {"sources": [{"serviceAccountToken": {"path": "token", "expirationSeconds": 3607}}, {"configMap": {"name": "kube-root-ca.crt", "items": [{"key": "ca.crt", "path": "ca.crt"}]}}, {"downwardAPI": {"items": [{"path": "namespace", "fieldRef": {"fieldPath": "metadata.namespace", "apiVersion": "v1"}}]}}, {"configMap": {"name": "openshift-service-ca.crt", "items": [{"key": "service-ca.crt", "path": "service-ca.crt"}]}}], "defaultMode": 420}}], "nodeName": "ip-10-30-218-170.ec2.internal", "priority": 0, "dnsPolicy": "ClusterFirst", "containers": [{"args": ["--config=/etc/otel/otelcol.yaml"], "name": "test-pod", "image": "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib@sha256:1720f9ce46441e0bb6e4b9ac448c476a950db0767fe774bb73877ecd46017dd7", "ports": [{"protocol": "TCP", "containerPort": 4317}, {"protocol": "TCP", "containerPort": 8889}], "resources": {"limits": {"cpu": "1", "memory": "2Gi"}, "requests": {"cpu": "200m", "memory": "100Mi"}}, "volumeMounts": [{"name": "otel-config", "subPath": "otelcol.yaml", "readOnly": true, "mountPath": "/etc/otel/otelcol.yaml"}, {"name": "kube-api-access-njsk9", "readOnly": true, "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"}], "livenessProbe": {"httpGet": {"path": "/", "port": 13133, "scheme": "HTTP"}, "periodSeconds": 10, "timeoutSeconds": 30, "failureThreshold": 30, "successThreshold": 1, "initialDelaySeconds": 1800}, "readinessProbe": {"httpGet": {"path": "/", "port": 13133, "scheme": "HTTP"}, "periodSeconds": 10, "timeoutSeconds": 30, "failureThreshold": 300, "successThreshold": 1, "initialDelaySeconds": 300}, "imagePullPolicy": "IfNotPresent", "securityContext": {"runAsUser": 1000930000, "capabilities": {"drop": ["ALL"]}, "runAsNonRoot": true, "allowPrivilegeEscalation": false}, "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}], "tolerations": [{"key": "node.kubernetes.io/not-ready", "effect": "NoExecute", "operator": "Exists", "tolerationSeconds": 300}, {"key": "node.kubernetes.io/unreachable", "effect": "NoExecute", "operator": "Exists", "tolerationSeconds": 300}, {"key": "node.kubernetes.io/memory-pressure", "effect": "NoSchedule", "operator": "Exists"}], "restartPolicy": "Always", "schedulerName": "default-scheduler", "serviceAccount": "default", "securityContext": {"fsGroup": 1000930000, "seLinuxOptions": {"level": "s0:c31,c0"}, "seccompProfile": {"type": "RuntimeDefault"}}, "imagePullSecrets": [{"name": "cpaas-container-registries"}, {"name": "default-dockercfg-rhb7z"}], "preemptionPolicy": "PreemptLowerPriority", "enableServiceLinks": true, "serviceAccountName": "default", "terminationGracePeriodSeconds": 30}, "status": {"phase": "Running", "podIP": "10.131.2.206", "hostIP": "10.30.218.170", "podIPs": [{"ip": "10.131.2.206"}], "qosClass": "Burstable", "startTime": "2024-04-05T09:57:32Z", "conditions": [{"type": "Initialized", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T09:57:32Z"}, {"type": "Ready", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T10:02:42Z"}, {"type": "ContainersReady", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T10:02:42Z"}, {"type": "PodScheduled", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T09:57:32Z"}], "containerStatuses": [{"name": "otel-collector", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci-widget-o6uljbey/opentelemetry-collector-contrib:0.64.1", "ready": true, "state": {"running": {"startedAt": "2024-04-05T09:57:34Z"}}, "imageID": "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib@sha256:1720f9ce46441e0bb6e4b9ac448c476a950db0767fe774bb73877ecd46017dd7", "started": true, "lastState": {}, "containerID": "cri-o://b6622cb6edcf8a9319771fd21c94d1796bc0d3a3f9b06c4cb44f154cadc0b06f", "restartCount": 0}]}, "metadata": {"uid": "42422d92-1a72-418d-97cf-97019c2d56e8", "name": "test-pod", "labels": {"app": "otelcollector", "otel-infra": "otel-pod", "pod-template-hash": "85fc74bc47"}, "namespace": "cpaas-ci", "annotations": {"openshift.io/scc": "restricted-v2", "k8s.v1.cni.cncf.io/network-status": "[{\n    \"name\": \"openshift-sdn\",\n    \"interface\": \"eth0\",\n    \"ips\": [\n        \"10.131.2.206\"\n    ],\n    \"default\": true,\n    \"dns\": {}\n}]", "seccomp.security.alpha.kubernetes.io/pod": "runtime/default", "alpha.image.policy.openshift.io/resolve-names": "*"}, "generateName": "otelcollector-85fc74bc47-", "ownerReferences": [{"uid": "852e6139-ad94-44e1-a813-f70b7ab1c033", "kind": "ReplicaSet", "name": "test-pod", "apiVersion": "apps/v1", "controller": true, "blockOwnerDeletion": true}], "resourceVersion": "1883964183", "creationTimestamp": "2024-04-05T09:57:32Z"} }`
)

var resourceQueryColumns = []string{"created_at", "id", "data"}
var tests = []struct {
	name     string
	database *Database
}{
	{
		name: "mariadb",
		database: &Database{
			selector:    MariaDBSelector{},
			filter:      MariaDBFilter{},
			sorter:      MariaDBSorter{},
			limiter:     MariaDBLimiter{},
			inserter:    MariaDBInserter{},
			deleter:     MariaDBDeleter{},
			paramParser: MariaDBParamParser{},
		},
	},
	{
		name: "postgresql",
		database: &Database{
			selector:    PostgreSQLSelector{},
			filter:      PostgreSQLFilter{},
			sorter:      PostgreSQLSorter{},
			limiter:     PostgreSQLLimiter{},
			inserter:    PostgreSQLInserter{},
			deleter:     PostgreSQLDeleter{},
			paramParser: &PostgreSQLParamParser{},
		},
	},
}

var subtests = []struct {
	name         string
	data         bool
	numResources int
}{
	{
		name:         "Results query",
		data:         true,
		numResources: 1,
	},
	{
		name:         "No results query",
		data:         false,
		numResources: 0,
	},
}

func NewMock() (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		slog.Error("an error '%s' was not expected when opening a stub database connection", "err", err)
		os.Exit(1)
	}

	return db, mock
}

func TestPodQueryLogURLs(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f1, _ := tt.database.filter.PodFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(1)
			f3, _ := tt.database.filter.NamespaceFilter(2)
			f4, _ := tt.database.filter.NameFilter(3)
			expectedQuery := regexp.QuoteMeta(
				fmt.Sprintf("%s WHERE %s AND %s AND %s AND %s",
					tt.database.selector.UrlFromResourceSelector(), f1, f2, f3, f4))
			db, mock := NewMock()
			tt.database.db = sqlx.NewDb(db, "sqlmock")

			rows := sqlmock.NewRows([]string{"url"})
			rows.AddRow("mock-url-container1")
			rows.AddRow("mock-url-container2")
			mock.ExpectQuery(expectedQuery).WithArgs(podApiVersion, namespace, podName).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logUrls, err := tt.database.QueryLogURLs(ctx, "Pod", podApiVersion, namespace, podName)
			assert.Equal(t, 2, len(logUrls))
			assert.NoError(t, err)
		})
	}
}

func TestLogURLsFromNonExistentResource(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = sqlx.NewDb(db, "sqlmock")
			rows := sqlmock.NewRows([]string{"uuid"})
			f1, _ := tt.database.filter.KindFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(2)
			f3, _ := tt.database.filter.NamespaceFilter(3)
			f4, _ := tt.database.filter.NameFilter(4)
			mock.ExpectQuery(regexp.QuoteMeta(
				fmt.Sprintf("%s WHERE %s AND %s AND %s AND %s",
					tt.database.selector.UUIDResourceSelector(), f1, f2, f3, f4)),
			).WithArgs(kind, cronJobApiVersion, namespace, cronJobName).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logUrls, err := tt.database.QueryLogURLs(ctx, kind, cronJobApiVersion, namespace, cronJobName)
			assert.Equal(t, 0, len(logUrls))
			assert.Error(t, err, "resource not found")
		})
	}
}

func TestCronJobQueryLogURLs(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = sqlx.NewDb(db, "sqlmock")

			// Get UUID query
			rows := sqlmock.NewRows([]string{"uuid"})
			rows.AddRow("mock-uuid-cronjob")
			f1, _ := tt.database.filter.KindFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(2)
			f3, _ := tt.database.filter.NamespaceFilter(3)
			f4, _ := tt.database.filter.NameFilter(4)
			mock.ExpectQuery(regexp.QuoteMeta(
				fmt.Sprintf("%s WHERE %s AND %s AND %s AND %s",
					tt.database.selector.UUIDResourceSelector(), f1, f2, f3, f4)),
			).WithArgs(kind, cronJobApiVersion, namespace, cronJobName).WillReturnRows(rows)

			ownerFilter, _ := tt.database.filter.OwnerFilter(1)
			ownedResourcesQuery := regexp.QuoteMeta(fmt.Sprintf("%s WHERE %s",
				tt.database.selector.OwnedResourceSelector(), ownerFilter))

			// Get owned job
			rows = sqlmock.NewRows([]string{"kind", "uuid"})
			rows.AddRow("Job", "mock-uuid-job")
			query, args, _ := tt.database.paramParser.ParseParams(ownedResourcesQuery, []string{"mock-uuid-cronjob"})
			mock.ExpectQuery(query).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			// Get owned pods
			rows = sqlmock.NewRows([]string{"kind", "uuid"})
			rows.AddRow("Pod", "mock-uuid-pod1")
			rows.AddRow("Pod", "mock-uuid-pod2")
			query, args, _ = tt.database.paramParser.ParseParams(ownedResourcesQuery, []string{"mock-uuid-job"})
			mock.ExpectQuery(query).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			// Get pods log urls
			rows = sqlmock.NewRows([]string{"url"})
			rows.AddRow("mock-log-url-pod1")
			rows.AddRow("mock-log-url-pod2")
			uuidFilter, _ := tt.database.filter.UuidFilter(1)
			query, args, _ = tt.database.paramParser.ParseParams(regexp.QuoteMeta(
				fmt.Sprintf("%s WHERE %s", tt.database.selector.UrlSelector(), uuidFilter)),
				[]string{"mock-uuid-pod1", "mock-uuid-pod2"})
			mock.ExpectQuery(query).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logUrls, err := tt.database.QueryLogURLs(ctx, kind, cronJobApiVersion, namespace, cronJobName)
			assert.Equal(t, 2, len(logUrls))
			assert.NoError(t, err)
		})
	}
}

func sliceOfAny2sliceOfValue(values []any) []driver.Value {
	var parsedValues []driver.Value
	for _, v := range values {
		parsedValues = append(parsedValues, v)
	}
	return parsedValues
}

func TestQueryResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f1, _ := tt.database.filter.KindFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(2)
			expectedQuery := regexp.QuoteMeta(fmt.Sprintf("%s WHERE %s AND %s %s %s",
				tt.database.selector.ResourceSelector(), f1, f2,
				tt.database.sorter.CreationTSAndIDSorter(),
				tt.database.limiter.Limiter(3)))
			for _, ttt := range subtests {
				t.Run(ttt.name, func(t *testing.T) {
					db, mock := NewMock()
					tt.database.db = sqlx.NewDb(db, "sqlmock")

					rows := sqlmock.NewRows(resourceQueryColumns)
					if ttt.data {
						rows.AddRow("2024-04-05T09:58:03Z", 5, json.RawMessage(testPodResource))
					}
					mock.ExpectQuery(expectedQuery).WithArgs(kind, podApiVersion, "100").WillReturnRows(rows)

					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()

					resources, lastId, _, err := tt.database.QueryResources(ctx, kind, version, "100", "", "")
					if ttt.numResources == 0 {
						assert.Nil(t, resources)
						assert.Equal(t, int64(0), lastId)
					} else {
						assert.NotNil(t, resources)
						assert.Equal(t, int64(5), lastId)
					}
					assert.Equal(t, ttt.numResources, len(resources))
					assert.NoError(t, err)
				})
			}
		})
	}
}

func TestQueryNamespacedResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f1, _ := tt.database.filter.KindFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(2)
			f3, _ := tt.database.filter.NamespaceFilter(3)
			expectedQuery := regexp.QuoteMeta(fmt.Sprintf("%s WHERE %s AND %s AND %s %s %s",
				tt.database.selector.ResourceSelector(), f1, f2, f3,
				tt.database.sorter.CreationTSAndIDSorter(),
				tt.database.limiter.Limiter(4)))
			for _, ttt := range subtests {
				t.Run(ttt.name, func(t *testing.T) {
					db, mock := NewMock()
					tt.database.db = sqlx.NewDb(db, "sqlmock")

					rows := sqlmock.NewRows(resourceQueryColumns)
					if ttt.data {
						rows.AddRow("2024-04-05T09:58:03Z", 1, json.RawMessage(testPodResource))
					}
					mock.ExpectQuery(expectedQuery).WithArgs(kind, podApiVersion, namespace, "100").WillReturnRows(rows)

					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()

					resources, _, _, err := tt.database.QueryNamespacedResources(ctx, kind, version, namespace, "100", "", "")
					if ttt.numResources == 0 {
						assert.Nil(t, resources)
					} else {
						assert.NotNil(t, resources)
					}
					assert.NoError(t, err)
				})
			}
		})
	}
}

func TestQueryNamespacedResourceByName(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f1, _ := tt.database.filter.KindFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(2)
			f3, _ := tt.database.filter.NamespaceFilter(3)
			f4, _ := tt.database.filter.NameFilter(4)
			expectedQuery := regexp.QuoteMeta(fmt.Sprintf("%s WHERE %s AND %s AND %s AND %s",
				tt.database.selector.ResourceSelector(), f1, f2, f3, f4))
			for _, ttt := range subtests {
				t.Run(ttt.name, func(t *testing.T) {
					db, mock := NewMock()
					tt.database.db = sqlx.NewDb(db, "sqlmock")

					rows := sqlmock.NewRows(resourceQueryColumns)
					if ttt.data {
						rows.AddRow("2024-04-05T09:58:03Z", 1, json.RawMessage(testPodResource))
					}
					mock.ExpectQuery(expectedQuery).WithArgs(kind, version, namespace, podName).WillReturnRows(rows)

					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()

					resource, err := tt.database.QueryNamespacedResourceByName(ctx, kind, version, namespace, podName)
					if ttt.numResources == 0 {
						assert.Nil(t, resource)
					} else {
						assert.NotNil(t, resource)
					}
					assert.NoError(t, err)
				})
			}
		})
	}
}

func TestQueryNamespacedResourceByNameMoreThanOne(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f1, _ := tt.database.filter.KindFilter(1)
			f2, _ := tt.database.filter.ApiVersionFilter(2)
			f3, _ := tt.database.filter.NamespaceFilter(3)
			f4, _ := tt.database.filter.NameFilter(4)
			expectedQuery := regexp.QuoteMeta(fmt.Sprintf("%s WHERE %s AND %s AND %s AND %s",
				tt.database.selector.ResourceSelector(), f1, f2, f3, f4))
			db, mock := NewMock()
			tt.database.db = sqlx.NewDb(db, "sqlmock")

			rows := sqlmock.NewRows(resourceQueryColumns).
				AddRow("2024-04-05T09:58:03Z", 1, json.RawMessage(testPodResource)).
				AddRow("2024-04-05T09:58:03Z", 2, json.RawMessage(testPodResource))
			mock.ExpectQuery(expectedQuery).WithArgs(kind, version, namespace, podName).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			resource, err := tt.database.QueryNamespacedResourceByName(ctx, kind, version, namespace, podName)
			assert.Nil(t, resource)
			assert.EqualError(t, err, "more than one resource found")
		})
	}
}

func TestPing(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.db = sqlx.NewDb(db, "sqlmock")
			mock.ExpectPing()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			assert.Nil(t, tt.database.Ping(ctx))
		})
	}
}

// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

const (
	kind              = "CronJob"
	cronJobApiVersion = "batch/v1"
	podKind           = "Pod"
	podApiVersion     = "v1"
	cronJobName       = "test-cronjob"
	podName           = "test-pod"
	version           = "v1"
	namespace         = "cpaas-ci"
	testPodResource   = `{"kind": "Pod", "apiVersion": "v1", "spec": {"volumes": [{"name": "otel-config", "configMap": {"name": "otel-collector-config", "items": [{"key": "otelcol.yaml", "path": "otelcol.yaml"}], "optional": true, "defaultMode": 420}}, {"name": "kube-api-access-njsk9", "projected": {"sources": [{"serviceAccountToken": {"path": "token", "expirationSeconds": 3607}}, {"configMap": {"name": "kube-root-ca.crt", "items": [{"key": "ca.crt", "path": "ca.crt"}]}}, {"downwardAPI": {"items": [{"path": "namespace", "fieldRef": {"fieldPath": "metadata.namespace", "apiVersion": "v1"}}]}}, {"configMap": {"name": "openshift-service-ca.crt", "items": [{"key": "service-ca.crt", "path": "service-ca.crt"}]}}], "defaultMode": 420}}], "nodeName": "ip-10-30-218-170.ec2.internal", "priority": 0, "dnsPolicy": "ClusterFirst", "containers": [{"args": ["--config=/etc/otel/otelcol.yaml"], "name": "test-pod", "image": "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib@sha256:1720f9ce46441e0bb6e4b9ac448c476a950db0767fe774bb73877ecd46017dd7", "ports": [{"protocol": "TCP", "containerPort": 4317}, {"protocol": "TCP", "containerPort": 8889}], "resources": {"limits": {"cpu": "1", "memory": "2Gi"}, "requests": {"cpu": "200m", "memory": "100Mi"}}, "volumeMounts": [{"name": "otel-config", "subPath": "otelcol.yaml", "readOnly": true, "mountPath": "/etc/otel/otelcol.yaml"}, {"name": "kube-api-access-njsk9", "readOnly": true, "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"}], "livenessProbe": {"httpGet": {"path": "/", "port": 13133, "scheme": "HTTP"}, "periodSeconds": 10, "timeoutSeconds": 30, "failureThreshold": 30, "successThreshold": 1, "initialDelaySeconds": 1800}, "readinessProbe": {"httpGet": {"path": "/", "port": 13133, "scheme": "HTTP"}, "periodSeconds": 10, "timeoutSeconds": 30, "failureThreshold": 300, "successThreshold": 1, "initialDelaySeconds": 300}, "imagePullPolicy": "IfNotPresent", "securityContext": {"runAsUser": 1000930000, "capabilities": {"drop": ["ALL"]}, "runAsNonRoot": true, "allowPrivilegeEscalation": false}, "terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File"}], "tolerations": [{"key": "node.kubernetes.io/not-ready", "effect": "NoExecute", "operator": "Exists", "tolerationSeconds": 300}, {"key": "node.kubernetes.io/unreachable", "effect": "NoExecute", "operator": "Exists", "tolerationSeconds": 300}, {"key": "node.kubernetes.io/memory-pressure", "effect": "NoSchedule", "operator": "Exists"}], "restartPolicy": "Always", "schedulerName": "default-scheduler", "serviceAccount": "default", "securityContext": {"fsGroup": 1000930000, "seLinuxOptions": {"level": "s0:c31,c0"}, "seccompProfile": {"type": "RuntimeDefault"}}, "imagePullSecrets": [{"name": "cpaas-container-registries"}, {"name": "default-dockercfg-rhb7z"}], "preemptionPolicy": "PreemptLowerPriority", "enableServiceLinks": true, "serviceAccountName": "default", "terminationGracePeriodSeconds": 30}, "status": {"phase": "Running", "podIP": "10.131.2.206", "hostIP": "10.30.218.170", "podIPs": [{"ip": "10.131.2.206"}], "qosClass": "Burstable", "startTime": "2024-04-05T09:57:32Z", "conditions": [{"type": "Initialized", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T09:57:32Z"}, {"type": "Ready", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T10:02:42Z"}, {"type": "ContainersReady", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T10:02:42Z"}, {"type": "PodScheduled", "status": "True", "lastProbeTime": null, "lastTransitionTime": "2024-04-05T09:57:32Z"}], "containerStatuses": [{"name": "otel-collector", "image": "image-registry.openshift-image-registry.svc:5000/cpaas-ci-widget-o6uljbey/opentelemetry-collector-contrib:0.64.1", "ready": true, "state": {"running": {"startedAt": "2024-04-05T09:57:34Z"}}, "imageID": "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib@sha256:1720f9ce46441e0bb6e4b9ac448c476a950db0767fe774bb73877ecd46017dd7", "started": true, "lastState": {}, "containerID": "cri-o://b6622cb6edcf8a9319771fd21c94d1796bc0d3a3f9b06c4cb44f154cadc0b06f", "restartCount": 0}]}, "metadata": {"uid": "42422d92-1a72-418d-97cf-97019c2d56e8", "name": "test-pod", "labels": {"app": "otelcollector", "otel-infra": "otel-pod", "pod-template-hash": "85fc74bc47"}, "namespace": "cpaas-ci", "annotations": {"openshift.io/scc": "restricted-v2", "k8s.v1.cni.cncf.io/network-status": "[{\n    \"name\": \"openshift-sdn\",\n    \"interface\": \"eth0\",\n    \"ips\": [\n        \"10.131.2.206\"\n    ],\n    \"default\": true,\n    \"dns\": {}\n}]", "seccomp.security.alpha.kubernetes.io/pod": "runtime/default", "alpha.image.policy.openshift.io/resolve-names": "*"}, "generateName": "otelcollector-85fc74bc47-", "ownerReferences": [{"uid": "852e6139-ad94-44e1-a813-f70b7ab1c033", "kind": "ReplicaSet", "name": "test-pod", "apiVersion": "apps/v1", "controller": true, "blockOwnerDeletion": true}], "resourceVersion": "1883964183", "creationTimestamp": "2024-04-05T09:57:32Z"} }`
	jsonPath          = "$."
	limit             = 100
)

var resourceQueryColumns = []string{"created_at", "id", "data"}
var tests = []struct {
	name     string
	database DBInterface
}{
	{
		name: "mariadb",
		database: &mariaDBDatabase{
			&DatabaseImpl{
				flavor:   sqlbuilder.MySQL,
				selector: mariaDBSelector{},
				filter:   mariaDBFilter{},
				sorter:   mariaDBSorter{},
				inserter: mariaDBInserter{},
				deleter:  facade.DBDeleterImpl{},
			},
		},
	},
	{
		name: "postgresql",
		database: &postgreSQLDatabase{
			&DatabaseImpl{
				flavor:   sqlbuilder.PostgreSQL,
				selector: postgreSQLSelector{},
				filter:   postgreSQLFilter{},
				sorter:   postgreSQLSorter{},
				inserter: postgreSQLInserter{},
				deleter:  facade.DBDeleterImpl{},
			},
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

func TestLogURLsFromNonExistentResource(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := tt.database.getFilter()
			db, mock := NewMock()
			tt.database.setConn(sqlx.NewDb(db, "sqlmock"))

			rows := sqlmock.NewRows([]string{"uuid"})
			sb := tt.database.getSelector().UUIDResourceSelector()
			sb.Where(
				filter.KindApiVersionFilter(sb.Cond, kind, cronJobApiVersion),
				filter.NamespaceFilter(sb.Cond, namespace),
				filter.NameFilter(sb.Cond, cronJobName),
			)
			query, _ := sb.BuildWithFlavor(tt.database.getFlavor())
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(kind, cronJobApiVersion, namespace, cronJobName).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logUrl, jp, err := tt.database.QueryLogURL(ctx, kind, cronJobApiVersion, namespace, cronJobName)
			assert.Equal(t, "", logUrl)
			assert.Equal(t, "", jp)
			assert.ErrorContains(t, err, "resource not found")
		})
	}
}

func TestCronJobQueryLogURLs(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.setConn(sqlx.NewDb(db, "sqlmock"))
			filter := tt.database.getFilter()
			selector := tt.database.getSelector()
			flavor := tt.database.getFlavor()

			// Get UUID query
			sb := selector.UUIDResourceSelector()
			sb = tt.database.getSorter().CreationTSAndIDSorter(sb)
			sb.Where(
				filter.KindApiVersionFilter(sb.Cond, kind, cronJobApiVersion),
				filter.NamespaceFilter(sb.Cond, namespace),
				filter.NameFilter(sb.Cond, cronJobName),
			)
			query, args := sb.BuildWithFlavor(flavor)

			rows := sqlmock.NewRows([]string{"uuid"})
			rows.AddRow("mock-uuid-cronjob")
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			// Get owned Job by the CronJob
			sb = selector.OwnedResourceSelector()
			sb.Where(filter.OwnerFilter(sb.Cond, []string{"mock-uuid-cronjob"}))
			query, args = sb.BuildWithFlavor(flavor)

			rows = sqlmock.NewRows([]string{"kind", "uuid"})
			rows.AddRow("Job", "mock-uuid-job")
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			// Get owned Pods by the Job
			sb = selector.OwnedResourceSelector()
			sb.Where(filter.OwnerFilter(sb.Cond, []string{"mock-uuid-job"}))
			query, args = sb.BuildWithFlavor(flavor)

			rows = sqlmock.NewRows([]string{"kind", "uuid"})
			rows.AddRow("Pod", "mock-uuid-pod1")
			rows.AddRow("Pod", "mock-uuid-pod2")
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			sb = selector.ResourceSelector()
			sb.Where(filter.UuidFilter(sb.Cond, "mock-uuid-pod1"))
			query, args = sb.BuildWithFlavor(flavor)

			rows = sqlmock.NewRows([]string{"created_at", "id", "data"})
			rows.AddRow("YYYY-MM-DDTHH:MM:SS+00", 0, testPodResource)
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			// Get pods log urls
			sb = selector.UrlSelector()
			sb.Where(
				filter.UuidFilter(sb.Cond, "42422d92-1a72-418d-97cf-97019c2d56e8"),
				filter.ContainerNameFilter(sb.Cond, "test-pod"),
			)
			query, args = sb.BuildWithFlavor(flavor)

			rows = sqlmock.NewRows([]string{"url", "json_path"})
			rows.AddRow("mock-log-url-pod1-container1", jsonPath)
			rows.AddRow("mock-log-url-pod1-container2", jsonPath)
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logUrl, jp, err := tt.database.QueryLogURL(ctx, kind, cronJobApiVersion, namespace, cronJobName)
			assert.NoError(t, err)
			assert.Equal(t, "mock-log-url-pod1-container1", logUrl)
			assert.Equal(t, jsonPath, jp)

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

func TestQueryResourcesWithoutNamespace(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := tt.database.getFilter()
			sb := tt.database.getSelector().ResourceSelector()
			sb.Where(
				filter.KindApiVersionFilter(sb.Cond, podKind, podApiVersion),
			)
			sb = tt.database.getSorter().CreationTSAndIDSorter(sb)
			sb.Limit(limit)
			query, _ := sb.BuildWithFlavor(tt.database.getFlavor())

			for _, ttt := range subtests {
				t.Run(ttt.name, func(t *testing.T) {
					db, mock := NewMock()
					tt.database.setConn(sqlx.NewDb(db, "sqlmock"))

					rows := sqlmock.NewRows(resourceQueryColumns)
					if ttt.data {
						rows.AddRow("2024-04-05T09:58:03Z", 5, json.RawMessage(testPodResource))
					}
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(podKind, podApiVersion, limit).WillReturnRows(rows)

					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()

					resources, lastId, _, err := tt.database.QueryResources(ctx, podKind, version,
						"", "", "", "", &LabelFilters{}, 100)
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

func TestQueryResources(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := tt.database.getFilter()
			sb := tt.database.getSelector().ResourceSelector()
			sb.Where(
				filter.KindApiVersionFilter(sb.Cond, podKind, podApiVersion),
				filter.NamespaceFilter(sb.Cond, namespace),
			)
			sb = tt.database.getSorter().CreationTSAndIDSorter(sb)
			sb.Limit(limit)
			query, _ := sb.BuildWithFlavor(tt.database.getFlavor())
			for _, ttt := range subtests {
				t.Run(ttt.name, func(t *testing.T) {
					db, mock := NewMock()
					tt.database.setConn(sqlx.NewDb(db, "sqlmock"))

					rows := sqlmock.NewRows(resourceQueryColumns)
					if ttt.data {
						rows.AddRow("2024-04-05T09:58:03Z", 1, json.RawMessage(testPodResource))
					}
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(podKind, podApiVersion, namespace, limit).WillReturnRows(rows)

					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()

					resources, _, _, err := tt.database.QueryResources(ctx, podKind, version, namespace,
						"", "", "", &LabelFilters{}, 100)
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

type arrayArg struct {
	args [][]map[string]string
}

// Match is a custom validator function to test if the arguments are equal without considering the order
func (arrayArgs arrayArg) Match(v driver.Value) bool {
	var match bool
	for _, arg := range arrayArgs.args {
		for _, elem := range arg {
			argValue, err := json.Marshal(elem)
			if err != nil {
				return false
			}
			argValueStr := strings.ReplaceAll(string(argValue), "\"", "\\\"")
			if strings.Contains(v.(string), argValueStr) {
				match = true
			} else {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	// Check existence filter for the not-in query
	for _, arg := range arrayArgs.args {
		for _, elem := range arg {
			for k := range elem {
				if !strings.Contains(v.(string), k) {
					return false
				}
				match = true
			}
		}
	}
	return match
}

func TestQueryResourcesWithLabelFilters(t *testing.T) {

	// NOTE: The extra values are commented because they make the unit tests flaky
	// The reason behind is that the order of arguments in a map is not deterministic
	var filterTests = []struct {
		name         string
		labelFilters LabelFilters
		args         *arrayArg
	}{
		{
			name: "existence", // kubectl get pods -l 'key1, key2'
			labelFilters: LabelFilters{
				Exists: []string{"key1", "key2"},
			},
		},
		{
			name: "not-existence", // kubectl get pods -l '!key1, !key2'
			labelFilters: LabelFilters{
				NotExists: []string{"key1", "key2"},
			},
		},
		{
			name: "equality", // kubectl get pods -l 'key1=value1,key2=value2'
			labelFilters: LabelFilters{
				Equals: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
		{
			name: "inequality", // kubectl get pods -l 'key1!=value1,key2!=value2'
			labelFilters: LabelFilters{
				NotEquals: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			args: &arrayArg{[][]map[string]string{{{"key1": "value1"}, {"key2": "value2"}}}},
		},
		{
			name: "set based", // kubectl get pods -l 'key1 in (value1, value3), key2 in (value2)'
			labelFilters: LabelFilters{
				In: map[string][]string{
					"key1": {"value1", "value3"},
					"key2": {"value2"},
				},
			},
			args: &arrayArg{[][]map[string]string{{{"key1": "value1"}, {"key1": "value3"}}, {{"key2": "value2"}}}},
		},
		{
			name: "set not based", // kubectl get pods -l 'key1 notin (value1, value3), key2 notin (value2)'
			labelFilters: LabelFilters{
				NotIn: map[string][]string{
					"key1": {"value1", "value3"},
					"key2": {"value2"},
				},
			},
			args: &arrayArg{[][]map[string]string{{{"key1": "value1"}, {"key1": "value3"}}, {{"key2": "value2"}}}},
		},
		{
			name: "all filters", // kubectl get pods -l 'key1, !key2, key3=value3, key4!=value4, key5 in (value5,value6), key6 notin (value6)'
			labelFilters: LabelFilters{
				Exists:    []string{"key1"},
				NotExists: []string{"key2"},
				Equals:    map[string]string{"key3": "value3"},
				NotEquals: map[string]string{"key4": "value4"},
				In:        map[string][]string{"key5": {"value5, value6"}},
				NotIn:     map[string][]string{"key6": {"value6"}},
			},
		},
	}

	for _, tt := range tests {
		for _, ttt := range filterTests {
			t.Run(fmt.Sprintf("%s %s", tt.name, ttt.name), func(t *testing.T) {
				filter := tt.database.getFilter()
				sb := tt.database.getSelector().ResourceSelector()
				sb.Where(
					filter.KindApiVersionFilter(sb.Cond, podKind, podApiVersion),
				)
				if ttt.labelFilters.Exists != nil {
					sb.Where(filter.ExistsLabelFilter(sb.Cond, ttt.labelFilters.Exists))
				}
				if ttt.labelFilters.NotExists != nil {
					sb.Where(filter.NotExistsLabelFilter(sb.Cond, ttt.labelFilters.NotExists))
				}
				if ttt.labelFilters.Equals != nil {
					sb.Where(filter.EqualsLabelFilter(sb.Cond, ttt.labelFilters.Equals))
				}
				if ttt.labelFilters.NotEquals != nil {
					sb.Where(filter.NotEqualsLabelFilter(sb.Cond, ttt.labelFilters.NotEquals))
				}
				if ttt.labelFilters.In != nil {
					sb.Where(filter.InLabelFilter(sb.Cond, ttt.labelFilters.In))
				}
				if ttt.labelFilters.NotIn != nil {
					sb.Where(filter.NotInLabelFilter(sb.Cond, ttt.labelFilters.NotIn))
				}
				sb = tt.database.getSorter().CreationTSAndIDSorter(sb)
				sb.Limit(100)
				query, args := sb.BuildWithFlavor(tt.database.getFlavor())
				db, mock := NewMock()
				tt.database.setConn(sqlx.NewDb(db, "sqlmock"))
				rows := sqlmock.NewRows(resourceQueryColumns)
				rows.AddRow("2024-04-05T09:58:03Z", 1, json.RawMessage(testPodResource))
				// In inequality, set-based and set not based, the order of the arguments is not ensured
				// That's why there is a custom validator function for this arguments
				if ttt.args != nil {
					expectedArgs := []driver.Value{podKind, podApiVersion}
					for i := 2; i < len(args)-1; i++ {
						expectedArgs = append(expectedArgs, ttt.args)
					}
					expectedArgs = append(expectedArgs, limit)
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(expectedArgs...).WillReturnRows(rows)
				} else {
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()

				resources, _, _, err := tt.database.QueryResources(ctx, podKind, version,
					"", "", "", "",
					&ttt.labelFilters, 100,
				)
				assert.NotNil(t, resources)
				assert.NoError(t, err)
			})
		}
	}

}

func TestQueryNamespacedResourceByName(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := tt.database.getFilter()
			sb := tt.database.getSelector().ResourceSelector()
			sb.Where(
				filter.KindApiVersionFilter(sb.Cond, kind, podApiVersion),
				filter.NamespaceFilter(sb.Cond, namespace),
				filter.NameFilter(sb.Cond, podName),
			)
			query, _ := sb.BuildWithFlavor(tt.database.getFlavor())
			for _, ttt := range subtests {
				t.Run(ttt.name, func(t *testing.T) {
					db, mock := NewMock()
					tt.database.setConn(sqlx.NewDb(db, "sqlmock"))

					rows := sqlmock.NewRows(resourceQueryColumns)
					if ttt.data {
						rows.AddRow("2024-04-05T09:58:03Z", 1, json.RawMessage(testPodResource))
					}
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(kind, version, namespace, podName).WillReturnRows(rows)

					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()

					resources, _, _, err := tt.database.QueryResources(ctx, kind, version, namespace, podName,
						"", "", &LabelFilters{}, 100)
					if ttt.numResources == 0 {
						assert.Empty(t, resources)
					} else {
						assert.NotEmpty(t, resources)
					}
					assert.NoError(t, err)
				})
			}
		})
	}
}

func TestPing(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := NewMock()
			tt.database.setConn(sqlx.NewDb(db, "sqlmock"))
			mock.ExpectPing()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			assert.Nil(t, tt.database.Ping(ctx))
		})
	}
}

func TestQueryLogUrlContainerDefault(t *testing.T) {
	innerTests := []struct {
		name                  string
		expectedContainerName string
		expectedLogUrl        string
	}{
		{
			name:                  "default container",
			expectedContainerName: "generate1",
			expectedLogUrl:        "https://logging.example.com/generate2",
		},
		{
			name:                  "uses annotation",
			expectedContainerName: "generate2",
			expectedLogUrl:        "https://logging.example.com/generate2",
		},
	}

	for _, tt := range tests {
		for _, innerTest := range innerTests {
			t.Run(innerTest.name, func(t *testing.T) {
				db, mock := NewMock()
				filter := tt.database.getFilter()
				tt.database.setConn(sqlx.NewDb(db, "sqlmock"))

				file, err := os.Open("testdata/pod-3-containers.json")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					file.Close()
				})

				podBytes, err := io.ReadAll(file)
				if err != nil {
					t.Fatal(err)
				}

				var pod corev1.Pod
				err = json.Unmarshal(podBytes, &pod)
				if err != nil {
					t.Fatal(err)
				}

				if innerTest.expectedContainerName == "generate2" {
					pod.SetAnnotations(map[string]string{defaultContainerAnnotation: innerTest.expectedContainerName})
				}

				newPodBytes, err := json.Marshal(pod)
				if err != nil {
					t.Fatal(err)
				}

				// Get the pod
				sb := tt.database.getSelector().ResourceSelector()
				sb = tt.database.getSorter().CreationTSAndIDSorter(sb)
				sb.Where(
					filter.KindApiVersionFilter(sb.Cond, pod.Kind, pod.APIVersion),
					filter.NamespaceFilter(sb.Cond, pod.Namespace),
					filter.NameFilter(sb.Cond, pod.Name),
				)
				query, args := sb.BuildWithFlavor(tt.database.getFlavor())

				rows := sqlmock.NewRows([]string{"created_at", "id", "data"})
				rows.AddRow("YYYY-MM-DDTHH:MM:SS+00", "0", string(newPodBytes))
				mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

				// Get the Logs
				sb = tt.database.getSelector().UrlSelector()
				sb.Where(
					filter.UuidFilter(sb.Cond, string(pod.UID)),
					filter.ContainerNameFilter(sb.Cond, innerTest.expectedContainerName),
				)
				query, args = sb.BuildWithFlavor(tt.database.getFlavor())
				rows = sqlmock.NewRows([]string{"url", "json_path"})
				rows.AddRow(innerTest.expectedLogUrl, "")
				mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)

				logUrl, jp, err := tt.database.QueryLogURL(context.Background(), pod.Kind, pod.APIVersion, pod.Namespace, pod.Name)
				assert.NoError(t, err)
				assert.Equal(t, innerTest.expectedLogUrl, logUrl)
				assert.Equal(t, "", jp)
			})
		}
	}
}

func TestWriteResource(t *testing.T) {
	innerTests := []struct {
		name    string
		inserts []struct {
			string
			time.Time
		}
		err error
	}{
		{
			name: "insert objects successfully",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"testdata/pod-3-containers.json",
					time.Now(),
				},
				{
					"testdata/job.json",
					time.Now(),
				},
			},
			err: nil,
		},
		{
			name: "insert and update object",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"testdata/pod-3-containers.json",
					time.Now(),
				},
				{
					"testdata/pod-3-containers.json",
					time.Now(),
				},
			},
			err: nil,
		},
		{
			name: "insert twice with no update due to older resource",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"testdata/pod-3-containers.json",
					time.Now(),
				},
				{
					"testdata/pod-3-containers.json",
					time.Time{},
				},
			},
			err: nil,
		},
		{
			name: "handle write failure",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"testdata/pod-3-containers.json",
					time.Now(),
				},
			},
			err: errors.New("error writing to the database"),
		},
	}
	for _, tt := range tests {
		for _, innerTest := range innerTests {
			t.Run(fmt.Sprintf("%s (db: %s)", innerTest.name, tt.name), func(t *testing.T) {
				for _, insert := range innerTest.inserts {
					db, mock := NewMock()
					tt.database.setConn(sqlx.NewDb(db, "sqlmock"))
					data, err := os.ReadFile(insert.string)
					if err != nil {
						t.Fatal(err)
					}
					obj, err := models.UnstructuredFromByteSlice(data)
					if err != nil {
						t.Fatal(err)
					}
					query, args := tt.database.getInserter().ResourceInserter(
						string(obj.GetUID()),
						obj.GetAPIVersion(),
						obj.GetKind(),
						obj.GetName(),
						obj.GetNamespace(),
						obj.GetResourceVersion(),
						insert.Time,
						sql.NullString{
							Valid: false,
						},
						data,
					).BuildWithFlavor(tt.database.getFlavor())
					mock.ExpectBegin()
					if innerTest.err == nil {
						mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectCommit()
					} else {
						mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(args).WillReturnError(innerTest.err)
						mock.ExpectRollback()
					}
					dbErr := tt.database.WriteResource(t.Context(), obj, data, insert.Time)
					if innerTest.err == nil {
						assert.Nil(t, dbErr)
					} else {
						assert.NotNil(t, dbErr)
					}
				}
			})
		}
	}
}

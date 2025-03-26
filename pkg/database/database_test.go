// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package database

import (
	"database/sql"
	"log/slog"
	"os"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/huandu/go-sqlbuilder"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
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

var tests = []struct {
	name     string
	database Database
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

func NewMock() (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		slog.Error("an error '%s' was not expected when opening a stub database connection", "err", err)
		os.Exit(1)
	}

	return db, mock
}

#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
# WARNING: this is intended to run in a single node
#   Kubernetes environment and just for testing purposes

set -o errexit
set -o xtrace

kubectl apply -f - <<EOF
---
apiVersion: v1
kind: Namespace
metadata:
  name: databases
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  labels:
    app: postgresql
  name: postgresql
  namespace: databases
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    type: local
    app: postgresql
  name: postgresql
spec:
  capacity:
    storage: 5Gi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: /data/postgresql
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: postgresql
  name: postgresql
  namespace: databases
spec:
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
        - name: postgresql
          image: postgres:16
          ports:
            - containerPort: 5432
          volumeMounts:
            - name: postgresql
              mountPath: /var/lib/postgresql/data
          env:
            - name: POSTGRES_DB
              value: root
            - name: POSTGRES_USER
              value: root
            - name: POSTGRES_PASSWORD
              value: password  # notsecret
      volumes:
        - name: postgresql
          persistentVolumeClaim:
            claimName: postgresql
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: postgresql
  name: postgresql
  namespace: databases
spec:
  type: ClusterIP
  ports:
    - port: 5432
  selector:
    app: postgresql

EOF

kubectl rollout status deployment --namespace=databases --timeout=30s
sleep 15  # Wait for PostgreSQL to be really ready

export DATABASE_POD=$(kubectl get -n databases pod -l app=postgresql -o name)
echo "CREATE USER kubearchive WITH ENCRYPTED PASSWORD 'P0stgr3sdbP@ssword';" | kubectl exec -i -n databases ${DATABASE_POD} -- psql -h localhost
echo "CREATE DATABASE kubearchive WITH OWNER kubearchive;" | kubectl exec -i -n databases ${DATABASE_POD} -- psql -h localhost
cat database/ddl-resource.sql | kubectl exec -i -n databases ${DATABASE_POD} -- psql -h localhost --user=kubearchive kubearchive


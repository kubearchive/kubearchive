# Generating the KubeArchive schema file.

Accessing the PostgreSQL database from outside the cluster requires port forwarding.  This
command should be run in its own terminal:
```
kubectl -n postgres port-forward service/kubearchive-rw 5433:5432 &
```
The local port (5433) in this case can be changed, but whatever value is chosen
needs to be used in the following command.

The PostgreSQL `kubearchive.sql` schema file in this directory is automatically
generated using the following command in a KubeArchive development environment:
```
pg_dump -h localhost -p 5433 -U kubearchive -C -c --if-exists -s
```
This file should always represent the current KubeArchive database schema. Upgrades will
be handled separately.

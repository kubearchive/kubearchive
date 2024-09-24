# Generating the KubeArchive schema file.

Accessing the MySQL database from outside the cluster requires port forwarding.
This command should be run in its own terminal:
```
kubectl -n mysql port-forward service/kubearchive 3307:3306
```
The local port (3307) in this case can be changed, but whatever value is chosen
needs to be used in the following command.

The MySQL `kubearchive.sql` schema file in this directory is automatically generated using the
following command in a KubeArchive development environment:
```
mysqldump -u root -h 127.0.0.1 -P 3307 -p --add-drop-table --add-drop-database --add-drop-trigger -B kubearchive --no-data --set-gtid-purged=OFF
```

This file should always represent the current KubeArchive database schema. Upgrades will
be handled separately.

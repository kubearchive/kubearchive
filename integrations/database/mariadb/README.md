# Generating the KubeArchive Schema File.

Accessing the MariaDB database from outside the cluster requires port forwarding.
This command should be run in its own terminal:
```
kubectl -n mariadb port-forward service/kubearchive 3307:3306
```
The local port (3307) in this case can be changed, but whatever value is chosen
needs to be used in the following command.

The MariaDB `kubearchive.sql` schema file in this directory is automatically generated using the
following command in a KubeArchive development environment:
```
mariadb-dump -u root -h localhost -P 3307 -p --add-drop-table --add-drop-database --add-drop-trigger -B kubearchive --no-data
```

This file should always represent the current KubeArchive database schema. Upgrades will
be handled separately.

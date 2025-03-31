# Generating the Kronicler schema file.

Accessing the MySQL database from outside the cluster requires port forwarding.
This command should be run in its own terminal:
```
kubectl -n mysql port-forward service/kronicler 3307:3306
```
The local port (3307) in this case can be changed, but whatever value is chosen
needs to be used in the following command.

The MySQL `kronicler.sql` schema file in this directory is automatically generated using the
following command in a Kronicler development environment:
```
mysqldump -u root -h 127.0.0.1 -P 3307 -p --add-drop-table --add-drop-database --add-drop-trigger -B kronicler --no-data --set-gtid-purged=OFF
```

This file should always represent the current Kronicler database schema. Upgrades will
be handled separately.

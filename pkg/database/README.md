# Supporting a New Database

## Code Changes
Supporting a new database is straightforward. In a new source file named <database>.go,
add the following:

1. Import the database driver.
1. Implement the DBInfoInterface (defined in `database.go`).
1. Implement the DBInterface (`defined in database.go`), overriding any functions as necessary
   to support this new database.
1. Implement an init function that registers your database.

It may be easiest to copy the PostgreSQL integration (`postgresql.go`) and use that
as a starting point.

## Add to the New Database to Helm

To configure helm support for the new database, modify the following:

1. Add the new key under `database` in `charts/kubearchive/values.yaml` for the database and
   provide values for subkeys named `dbUrl` and `dbPort`.
1. Add logic in `charts/kubearchive/templates/database/database_secret.yaml` to insert
   the values for `DATABASE_URL` and `DATABASE_PORT`, referencing the new values
   added to `charts/kubearchive/values.yaml` in the previous step.

## Change the Database used by Helm

To change the database used by KubeArchive, change the value of the key `database.kind`
in `charts/kubearchive/values.yaml`. This value must match the value used when registering
the database in KubeArchive.

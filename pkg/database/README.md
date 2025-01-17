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

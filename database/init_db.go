// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"os"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "" // the db_user from postgres-secret.yaml, ie: "admin", "ps_user".
	password = "" // the db_password from postgres-secret.yaml
	dbname   = "" // the db_name from postgres-secret.yaml, ie: "postgres", "test_db".
)

func main() {
	// connect to the DB.
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	// postgres is the driver type.
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	defer db.Close()

	// SQL instruction to create a table.
	sqlStatement := `
	CREATE TABLE public.test_objects (
		"id" serial PRIMARY KEY,
		"api_version" varchar NOT NULL,
		"kind" varchar NOT NULL,
		"name" varchar NOT NULL,
		"namespace" varchar NOT NULL,
		"resource_version" varchar NULL,
		"created_ts" timestamp NOT NULL,
		"updated_ts" timestamp NOT NULL,
		"data" jsonb NOT NULL
	);
	`
	_, err = db.Exec(sqlStatement)
	if err != nil {
		panic(err)
	}
	fmt.Println("table test_objects created in the DB.")

	// load the test data from file
	testData := "test_objects.sql"
	//fmt.Println(testData)
	query, err := os.ReadFile(testData)
	if err != nil {
		panic(err)
	}

	// insert the data into the table.
	_, err = db.Exec(string(query))
	if err != nil {
		panic(err)
	}
	fmt.Println("testdata from test_objects.sql inserted in the DB.")
}

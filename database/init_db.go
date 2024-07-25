// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

type flags struct {
	DatabaseName     string
	DatabaseUser     string
	DatabasePassword string
}

var defaultValues = &flags{
	DatabaseName:     "postgresdb",
	DatabaseUser:     "ps_user",
	DatabasePassword: "P0stgr3sdbP@ssword", // notsecret
}

const (
	host = "localhost"
	port = 5432
)

func main() {
	var flagValues flags
	flag.StringVar(&flagValues.DatabaseName, "database-name", defaultValues.DatabaseName, "PostgreSQL database name")
	flag.StringVar(&flagValues.DatabaseUser, "database-user", defaultValues.DatabaseUser, "PostgreSQL database user")
	flag.StringVar(&flagValues.DatabasePassword, "database-password", defaultValues.DatabasePassword, "PostgreSQL database password")
	flag.Parse()

	// connect to the DB.
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, flagValues.DatabaseUser, flagValues.DatabasePassword, flagValues.DatabaseName)

	// postgres is the driver type.
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	defer db.Close()

	// pgSQL instruction to create the triggers for CREATED_AT and UPDATED_AT
	pgSQLTriggerTSFunction := `
	CREATE OR REPLACE FUNCTION trigger_set_timestamp()
	RETURNS TRIGGER AS $$
	BEGIN
	  NEW.updated_at = NOW();
	  RETURN NEW;
	  END;
	$$ LANGUAGE plpgsql;
	`
	_, err = db.Exec(pgSQLTriggerTSFunction)
	if err != nil {
		panic(err)
	}
	fmt.Println("trigger_set_timestamp() function created in db")

	// SQL instruction to create a table.
	sqlStatement := `
	CREATE TABLE IF NOT EXISTS public.resource (
		"uuid" uuid PRIMARY KEY,
		"api_version" varchar NOT NULL,
		"kind" varchar NOT NULL,
		"name" varchar NOT NULL,
		"namespace" varchar NOT NULL,
		"resource_version" varchar NULL,
		"created_at" timestamp NOT NULL DEFAULT now(),
		"updated_at" timestamp NOT NULL DEFAULT now(),
		"cluster_deleted_ts" timestamp NULL,
		"data" jsonb NOT NULL
	);
	`
	_, err = db.Exec(sqlStatement)
	if err != nil {
		panic(err)
	}
	fmt.Println("table resource created in the DB.")

	// SQL instruction to create the trigger of the table
	sqlTrigger := `
	CREATE OR REPLACE TRIGGER set_timestamp
	BEFORE UPDATE ON public.resource
	FOR EACH ROW
	EXECUTE PROCEDURE trigger_set_timestamp();
	`
	_, err = db.Exec(sqlTrigger)
	if err != nil {
		panic(err)
	}
	fmt.Println("trigger created in the DB.")

	// load the test data from file
	testData := "database/resource.sql"
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
	fmt.Println("testdata from resource.sql inserted in the DB.")
}

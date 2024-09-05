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
	DatabaseName:     "kubearchive",
	DatabaseUser:     "kubearchive",
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

	// load the DDL schema from file
	ddlFile, err := os.ReadFile("database/ddl.sql")
	if err != nil {
		panic(err)
	}

	// run the DDL instructions
	_, err = db.Exec(string(ddlFile))
	if err != nil {
		panic(err)
	}
	fmt.Println("Schema from ddl.sql set in the DB")

	// load the test data from file
	testData := "database/dml-example.sql"
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
	fmt.Println("testdata from dml-example.sql inserted in the DB.")
}

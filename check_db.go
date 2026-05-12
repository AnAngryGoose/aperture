package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "aperture.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT mount, used, total FROM disk_mount_metrics LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var mount string
		var used, total int64
		if err := rows.Scan(&mount, &used, &total); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Mount: %s, Used: %d, Total: %d\n", mount, used, total)
	}
}

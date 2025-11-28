package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "data/traffic.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check if column exists
	rows, err := db.Query("PRAGMA table_info(user_settings)")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			log.Fatal(err)
		}
		if name == "enable_short_link" {
			hasColumn = true
			break
		}
	}

	if hasColumn {
		fmt.Println("Column enable_short_link already exists")
		return
	}

	// Add column
	_, err = db.Exec("ALTER TABLE user_settings ADD COLUMN enable_short_link INTEGER NOT NULL DEFAULT 0")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Successfully added enable_short_link column to user_settings table")
}

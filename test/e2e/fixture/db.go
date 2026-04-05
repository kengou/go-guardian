package main

import (
	"database/sql"
	"fmt"
)

// INTENTIONAL: SQL injection via string concatenation (OWASP A03).
// go-guardian's check_owasp should flag the fmt.Sprintf with SELECT.
// Do NOT fix this — it exists for e2e testing.

func HandleQuery(userInput string) string {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return "error"
	}
	defer db.Close()

	query := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", userInput)
	rows, _ := db.Query(query)
	if rows != nil {
		rows.Close()
	}

	return "ok"
}

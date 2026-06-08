package main

import (
	"log"
	"os"

	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/server"
)

func main() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/library_db?sslmode=disable"
	}

	db, err := database.ConnectAndMigrate(dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	store := database.NewStore(db)

	srv := server.New(store, ":8080")
	log.Fatal(srv.ListenAndServe())
}

package library

import (
	"log"
	"os"

	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/server"
)

type library struct {
	store *database.Store
	srv   *server.Server
}

func Init() *library {
	lib := library{}
	return &lib
}

func (l *library) InitDB() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		host := os.Getenv("DB_HOST")
		port := os.Getenv("DB_PORT")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		dbName := os.Getenv("DB_NAME")
		if host == "" {
			host = "localhost"
		}
		if port == "" {
			port = "5432"
		}
		if user == "" {
			user = "postgres"
		}
		if password == "" {
			password = "postgres"
		}
		if dbName == "" {
			dbName = "library_db"
		}
		dsn = "postgres://" + user + ":" + password + "@" + host + ":" + port + "/" + dbName + "?sslmode=disable"
	}

	db, err := database.ConnectAndMigrate(dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	l.store = database.NewStore(db)
}

func (l *library) InitServer() {
	l.srv = server.New(l.store, ":8080")
	log.Fatal(l.srv.ListenAndServe())
}

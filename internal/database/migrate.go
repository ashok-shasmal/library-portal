package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// ConnectDB opens a Postgres connection and verifies it with retry logic.
// It attempts to connect up to 30 times with exponential backoff.
func ConnectDB(conn string) (*sql.DB, error) {
	maxRetries := 30
	baseDelay := time.Second
	maxDelay := 10 * time.Second

	var db *sql.DB
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		db, err = sql.Open("postgres", conn)
		if err != nil {
			return nil, fmt.Errorf("open db: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr := db.PingContext(ctx)
		defer cancel()

		if pingErr == nil {
			log.Printf("Successfully connected to database after %d attempt(s)", attempt+1)
			return db, nil
		}

		db.Close()

		if attempt < maxRetries-1 {
			// Calculate backoff with exponential increase: 1s, 2s, 4s, 8s, up to 10s
			delay := baseDelay * time.Duration(1<<uint(attempt))
			if delay > maxDelay {
				delay = maxDelay
			}
			log.Printf("Database connection failed (attempt %d/%d): %v. Retrying in %v...", attempt+1, maxRetries, pingErr, delay)
			time.Sleep(delay)
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
}

// Migrate creates the necessary tables for the library portal.
func Migrate(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT NOT NULL UNIQUE,
            password TEXT NOT NULL,
            balance DOUBLE PRECISION NOT NULL DEFAULT 0,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now()
        );`,

		`CREATE TABLE IF NOT EXISTS books (
            id SERIAL PRIMARY KEY,
            title TEXT NOT NULL,
            author_name TEXT NOT NULL,
            is_available BOOLEAN NOT NULL DEFAULT true,
            rent_price DOUBLE PRECISION NOT NULL DEFAULT 0,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now()
        );`,

		`CREATE TABLE IF NOT EXISTS borrow_records (
            id SERIAL PRIMARY KEY,
            user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
            borrowed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            due_date TIMESTAMPTZ NOT NULL,
            returned_at TIMESTAMPTZ NULL,
            rent_paid DOUBLE PRECISION NOT NULL DEFAULT 0
        );`,
	}

	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	return nil
}

// ConnectAndMigrate is a convenience helper to open the DB and run migrations.
func ConnectAndMigrate(conn string) (*sql.DB, error) {
	db, err := ConnectDB(conn)
	if err != nil {
		return nil, err
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

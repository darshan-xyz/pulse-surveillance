package database

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB() error {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Println("DB_DSN not set, skipping Postgres initialization for mock mode")
		return nil // Graceful fallback if not set
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	if err := db.Ping(); err != nil {
		return err
	}

	DB = db
	log.Println("Successfully connected to PostgreSQL")
	return runMigrations(db)
}

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS homes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS home_members (
		home_id TEXT REFERENCES homes(id),
		user_id TEXT REFERENCES users(id),
		role TEXT NOT NULL,
		PRIMARY KEY(home_id, user_id)
	);

	CREATE TABLE IF NOT EXISTS devices (
		id TEXT PRIMARY KEY,
		home_id TEXT REFERENCES homes(id),
		name TEXT NOT NULL,
		public_key TEXT NOT NULL,
		status TEXT DEFAULT 'REGISTERED',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(schema)
	if err != nil {
		log.Printf("Failed to run database migrations: %v\n", err)
		return err
	}
	log.Println("Database schemas verified.")
	return nil
}

package database

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() error {
	dataDir := filepath.Join(".", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	dbPath := filepath.Join(dataDir, "hss.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS cameras (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		protocol TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL,
		username_encrypted TEXT,
		password_encrypted TEXT,
		main_stream_url_encrypted TEXT,
		sub_stream_url_encrypted TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS recordings (
		id TEXT PRIMARY KEY,
		camera_id TEXT NOT NULL REFERENCES cameras(id),
		started_at TEXT NOT NULL,
		ended_at TEXT NOT NULL,
		path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		protected INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_recordings_camera_time ON recordings(camera_id, started_at);

	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		camera_id TEXT NOT NULL REFERENCES cameras(id),
		type TEXT NOT NULL,
		started_at TEXT NOT NULL,
		ended_at TEXT,
		confidence REAL,
		recording_id TEXT REFERENCES recordings(id),
		thumbnail_path TEXT,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_events_camera_time ON events(camera_id, started_at);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	DB = db
	log.Println("Database initialized successfully.")
	return nil
}

package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	StatusIdle      = sql.NullString{String: "idle", Valid: true}
	StatusDeploying = sql.NullString{String: "deploying", Valid: true}
	StatusRunning   = sql.NullString{String: "running", Valid: true}
	StatusError     = sql.NullString{String: "error", Valid: true}
)

func GenerateAppID() string {
	return fmt.Sprintf("app_%d_%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}

func (q *Queries) CreateTables(ctx context.Context) error {
	createAppsTable := `
		CREATE TABLE IF NOT EXISTS apps (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			repo_url TEXT NOT NULL,
			language TEXT,
			port INTEGER,
			container_id TEXT,
			image_id TEXT,
			status TEXT DEFAULT 'idle',
			error_msg TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	if _, err := q.db.ExecContext(ctx, createAppsTable); err != nil {
		return fmt.Errorf("error creando tabla apps: %w", err)
	}
	return nil
}

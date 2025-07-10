package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "embed"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/apps.sql
var createAppsTable string

var (
	StatusIdle        = sql.NullString{String: "idle", Valid: true}
	StatusDeploying   = sql.NullString{String: "deploying", Valid: true}
	StatusRedeploying = sql.NullString{String: "redeploying", Valid: true}
	StatusRunning     = sql.NullString{String: "running", Valid: true}
	StatusError       = sql.NullString{String: "error", Valid: true}
)

func GenerateAppID() string {
	return fmt.Sprintf("app_%d_%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}

func (q *Queries) CreateTables(ctx context.Context) error {
	if _, err := q.db.ExecContext(ctx, createAppsTable); err != nil {
		return fmt.Errorf("error creando tabla apps: %v", err)
	}
	return nil
}

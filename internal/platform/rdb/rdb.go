package rdb

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Open は PostgreSQL へ接続し、疎通を確認してから返す。失敗時は接続を閉じてエラーを返す。
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return db, nil
}

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/migration"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/migrations"
)

// runMigrate は埋め込んだマイグレーションのうち未適用のものを DB_DSN の DB に適用する（`app migrate`）。
func runMigrate(ctx context.Context) error {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		return fmt.Errorf("DB_DSN is not set")
	}

	ms, err := migration.Load(migrations.FS)
	if err != nil {
		return err
	}

	db, err := rdb.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	applied, err := migration.Apply(ctx, db, ms)
	for _, a := range applied {
		slog.Info("Migration applied", "version", a.Version, "name", a.Name, "execution_ms", a.ExecutionMs)
	}
	if err != nil {
		return err
	}
	slog.Info("Migrations up to date", "applied", len(applied), "total", len(ms), "version", version)
	return nil
}

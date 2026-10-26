// -env で指定した環境変数のつなぎ先（-dbname で DB の名前だけを変えられる）の PostgreSQL に、種別の一覧（domaindef.Types）を合わせる。
// 本番・開発の core は起動時に合わせるが、マイグレーションを足した直後（make migrate）の開発の DB と、テストが使う DB（core_test・
// 複製するテンプレート core_template）は core を起動せずにマイグレーションを流すため、ここで合わせる（core#33）。開発・CI の道具で、本番のコード（cmd/app・internal）からは import しない
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func main() {
	env := flag.String("env", "", "つなぎ先を持つ環境変数の名前（例: STUDIO_DB_DSN）")
	dbname := flag.String("dbname", "", "合わせる DB の名前（指定が無ければつなぎ先のまま）")
	flag.Parse()
	if err := run(context.Background(), *env, *dbname); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, env, dbname string) error {
	dsn := os.Getenv(env)
	if dsn == "" {
		return fmt.Errorf("-env の環境変数のつなぎ先が要る")
	}
	// lib/pq の key=value 形式では、後に書いた値が使われる
	if dbname != "" {
		dsn += " dbname=" + dbname
	}
	db, err := rdb.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := domaindef.Sync(rdb.WithDB(ctx, db), domaindef.Types); err != nil {
		return err
	}
	slog.Info("synced domain types", "env", env, "dbname", dbname)
	return nil
}

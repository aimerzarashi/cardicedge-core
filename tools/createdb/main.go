// -env で指定した環境変数のつなぎ先の PostgreSQL に、指定した名前の DB が無ければ作る。テストが使う core_test と、複製するテンプレート（core_template）を、
// 開発（make test）と CI の両方で同じ方法で用意するために使う（CI のイメージには psql が無く、apt で入れると外のミラーに左右されるため）。
// 開発・CI の道具で、本番のコード（cmd/app・internal）からは import しない
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func main() {
	env := flag.String("env", "", "つなぎ先を持つ環境変数の名前（例: TEST_DB_DSN）")
	name := flag.String("name", "", "作る DB の名前")
	flag.Parse()
	if err := run(context.Background(), os.Getenv(*env), *name); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, dsn, name string) error {
	if dsn == "" || name == "" {
		return fmt.Errorf("-env の環境変数のつなぎ先と -name が要る")
	}
	// つなぎ先の DB（core_test 等）は、これから作るもののことがあるため、どのサーバーにもある postgres につなぐ
	// （lib/pq の key=value 形式では、後に書いた値が使われる）
	db, err := rdb.Open(ctx, dsn+" dbname=postgres")
	if err != nil {
		return err
	}
	defer db.Close()
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	// CREATE DATABASE は引数（$1）を使えないため、名前を識別子として引用して埋め込む
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name)); err != nil {
		return err
	}
	slog.Info("created database", "name", name)
	return nil
}

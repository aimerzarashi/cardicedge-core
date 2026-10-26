package rdbtest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// テストが使う DB のつなぎ先。core 本体の STUDIO_DB_DSN・GAME_DB_DSN とは分け、開発の core が使う DB（core_studio・core_game）に
// テストが触らないようにする（開発で入れたデータがテストの前提に混ざらないため。core#33）。make test（migrate-template）と CI が、
// このサーバーに core_test と複製の元のテンプレートを作り、マイグレーションと種別を流しておく
const envName = "TEST_DB_DSN"

// 外に出さないのは、共有の core_test に commit するテストを書けなくするため（行が残り、ほかのテストの前提を崩す）
func open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv(envName)
	if dsn == "" {
		t.Skip(envName + " is not set. run via 'make test'")
	}
	db, err := rdb.Open(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// テストの終わりに rollback するのは、共有の core_test にテストのデータを残さないため
func Tx(t *testing.T) context.Context {
	t.Helper()
	db := open(t)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { tx.Rollback() })
	return rdb.WithTx(rdb.WithDB(t.Context(), db), tx)
}

// make test（migrate-template）と CI がマイグレーションを流しておく、複製の元の DB
const templateName = "core_template"

var dbNamePattern = regexp.MustCompile(`dbname=\S+`)

// テンプレートから、このテストだけの DB を複製して、接続と接続先（DSN。自分で接続する Listener 等に渡す）を返し、テストの終わりに消す。
// 共有の DB を使わないのは、通知（NOTIFY）が DB の単位で届き、同時に動くほかのテストのイベントまで届くため（core#9）。
// 複製はマイグレーションを流すより速い（約 0.15 秒。core#9 で計測）
func NewDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	if os.Getenv(envName) == "" {
		t.Skip(envName + " is not set. run via 'make test'")
	}
	db, dsn, drop, err := CloneDatabase(t.Context())
	require.NoError(t, err)
	t.Cleanup(drop)
	return db, dsn
}

// NewDatabase と同じく複製する。testing.T が無いところ（結合テストの TestMain）で使うため、消す関数を返す
func CloneDatabase(ctx context.Context) (db *sql.DB, dsn string, drop func(), err error) {
	adminDSN := os.Getenv(envName)
	if adminDSN == "" {
		return nil, "", nil, errors.New(envName + " is not set. run via 'make test'")
	}
	if !dbNamePattern.MatchString(adminDSN) {
		return nil, "", nil, errors.New(envName + " に dbname= が無い: 複製した DB の接続先を作れない")
	}
	admin, err := rdb.Open(ctx, adminDSN)
	if err != nil {
		return nil, "", nil, err
	}
	name := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name)+" TEMPLATE "+templateName)
	if err != nil {
		admin.Close()
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "3D000" {
			return nil, "", nil, fmt.Errorf("テンプレート %s が無い。make test（migrate-template）で用意する", templateName)
		}
		return nil, "", nil, err
	}
	dropDatabase := func() {
		_, _ = admin.ExecContext(context.WithoutCancel(ctx), "DROP DATABASE IF EXISTS "+pq.QuoteIdentifier(name)+" WITH (FORCE)")
		admin.Close()
	}

	dsn = dbNamePattern.ReplaceAllString(adminDSN, "dbname="+name)
	db, err = rdb.Open(ctx, dsn)
	if err != nil {
		dropDatabase()
		return nil, "", nil, err
	}
	return db, dsn, func() {
		db.Close()
		dropDatabase()
	}, nil
}

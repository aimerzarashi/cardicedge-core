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

// DB_DSN の DB にはマイグレーションを適用済みであること。外に出さないのは、開発用の DB に commit するテストを書けなくするため
func open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN is not set. run via 'make test'")
	}
	db, err := rdb.Open(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// テストの終わりに rollback するのは、開発用の DB でテストしても、テストのデータを残さないため
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
	if os.Getenv("DB_DSN") == "" {
		t.Skip("DB_DSN is not set. run via 'make test'")
	}
	db, dsn, drop, err := CloneDatabase(t.Context())
	require.NoError(t, err)
	t.Cleanup(drop)
	return db, dsn
}

// NewDatabase と同じく複製する。testing.T が無いところ（結合テストの TestMain）で使うため、消す関数を返す
func CloneDatabase(ctx context.Context) (db *sql.DB, dsn string, drop func(), err error) {
	adminDSN := os.Getenv("DB_DSN")
	if adminDSN == "" {
		return nil, "", nil, errors.New("DB_DSN is not set. run via 'make test'")
	}
	if !dbNamePattern.MatchString(adminDSN) {
		return nil, "", nil, errors.New("DB_DSN に dbname= が無い: 複製した DB の接続先を作れない")
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

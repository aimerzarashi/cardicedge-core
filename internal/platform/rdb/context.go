package rdb

import (
	"context"
	"database/sql"
)

type dbContextKey struct{}

type txContextKey struct{}

func WithDB(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, dbContextKey{}, db)
}

func dbFromContext(ctx context.Context) (*sql.DB, bool) {
	db, ok := ctx.Value(dbContextKey{}).(*sql.DB)
	return db, ok
}

// 公開しているのは、Transact の外で始めたトランザクションに参加させるため（テストで最後に rollback して DB を汚さない等）
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}

// context なしの操作も含めるのは、sqlboiler の boil.ContextExecutor として渡すため
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// 返す口は、ctx のスパンの traceparent を SQL のコメントに足す（tracecomment.go）
func GetExecutor(ctx context.Context) (Executor, error) {
	if tx, ok := txFromContext(ctx); ok {
		return traceCommentExecutor{tx}, nil
	}
	if db, ok := dbFromContext(ctx); ok {
		return traceCommentExecutor{db}, nil
	}
	return nil, ErrNoDBConnection
}

// context に載った DB から、接続を 1 本だけ取り出す（使い終わったら Close でプールに返す）。接続に付く状態
// （アドバイザリーロック等）を同じ接続で取って外すためで、ふだんの読み書きは GetExecutor を使う
func Conn(ctx context.Context) (*sql.Conn, error) {
	db, ok := dbFromContext(ctx)
	if !ok {
		return nil, ErrNoDBConnection
	}
	return db.Conn(ctx)
}

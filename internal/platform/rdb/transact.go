package rdb

import (
	"context"
	"sync"
)

type afterCommitContextKey struct{}

// Transact が始めたトランザクションの、確定のあとに動かす処理を貯める
type afterCommitHooks struct {
	mu  sync.Mutex
	fns []func(context.Context)
}

// トランザクションが確定したあとに fn を動かす。取り消されたら動かさない（取り消された保存を記録に残さないため）。
// 参加している Transact の外側が確定したときに動く。Transact の外で始めたトランザクション（WithTx）は、
// 確定を知る手段が無いので動かさない（テストの最後の rollback と同じ扱い）。トランザクションが無ければすぐ動かす。
// ctx に載せる仕組みにしたのは、イベントストアの Save が呼ぶ側の Transact を知らないまま、確定のあとにログを出すため
// （Transact の戻り値や引数に処理を足す案は、全コマンドの呼び出しを変えるため採らなかった）
func AfterCommit(ctx context.Context, fn func(context.Context)) {
	if hooks, ok := ctx.Value(afterCommitContextKey{}).(*afterCommitHooks); ok {
		hooks.mu.Lock()
		hooks.fns = append(hooks.fns, fn)
		hooks.mu.Unlock()
		return
	}
	if _, ok := txFromContext(ctx); ok {
		return
	}
	fn(ctx)
}

// context にトランザクションがすでにあれば、新しく始めずに参加する。入れ子で別のトランザクションを始めると、
// 外側が rollback しても内側だけが commit されてしまうため（参照の api の不具合。設計メモ 6 節の 2）
func Transact[T any](ctx context.Context, fn func(context.Context) (T, error)) (result T, err error) {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	db, ok := dbFromContext(ctx)
	if !ok {
		return result, ErrNoDBConnection
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	// fn が panic しても、トランザクションを開いたまま接続をプールに返さない
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	hooks := &afterCommitHooks{}
	result, err = fn(context.WithValue(WithTx(ctx, tx), afterCommitContextKey{}, hooks))
	if err != nil {
		_ = tx.Rollback()
		var zero T
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		var zero T
		return zero, err
	}
	for _, fn := range hooks.fns {
		fn(ctx)
	}
	return result, nil
}

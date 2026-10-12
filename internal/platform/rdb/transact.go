package rdb

import "context"

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

	result, err = fn(WithTx(ctx, tx))
	if err != nil {
		_ = tx.Rollback()
		var zero T
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

package constraint

import (
	"context"
	"database/sql"
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 参照の api（constraint/avatar.go）と同じ形。同じ集約の行があれば更新し、ほかの集約が同じ key を使っていれば
// INSERT 自体を起こさず、行が返らないことで重複を知る（登録にも変更にも使える）。重複を一意制約違反のエラーで受けると、
// ふだんの重複のたびに PostgreSQL が ERROR のログを出し（重複は異常ではないのにログが埋まる）、トランザクションも中断されるため。
// ほかの人がまったく同時に同じ key にしたときだけ NOT EXISTS をすり抜けて一意制約違反になるが、それも重複として返す。
// sqlboiler の Upsert は、ほかの集約が使っているかの確かめ（NOT EXISTS）を書けないため、SQL を直接書く。
// key の種別は名前から引く（種別が無いと行が返らず重複と区別できないが、起動時に domaindef で照合している）
const updateUniqueKeyQuery = `
	INSERT INTO domain_unique_keys (unique_key_type_id, key, entity_id)
	SELECT t.id, $2, $3
	FROM domain_unique_key_types t
	WHERE t.name = $1
	  AND NOT EXISTS (SELECT 1 FROM domain_unique_keys u WHERE u.unique_key_type_id = t.id AND u.key = $2 AND u.entity_id != $3)
	ON CONFLICT (unique_key_type_id, entity_id) DO UPDATE SET key = EXCLUDED.key
	RETURNING entity_id
`

func updateUniqueKey(ctx context.Context, entityID int64, key domain.UniqueKey, inUse error) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	var updated int64
	err = exec.QueryRowContext(ctx, updateUniqueKeyQuery, key.Type().String(), key.Value(), entityID).Scan(&updated)
	if errors.Is(err, sql.ErrNoRows) || rdb.IsUniqueViolation(err) {
		return inUse
	}
	return err
}

const findUniqueKeyOwnerQuery = `
	SELECT u.entity_id
	FROM domain_unique_keys u
	JOIN domain_unique_key_types t ON t.id = u.unique_key_type_id
	WHERE t.name = $1 AND u.key = $2
`

// key を確保している集約の ID を返す
func findUniqueKeyOwner(ctx context.Context, key domain.UniqueKey) (int64, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, false, err
	}
	var entityID int64
	err = exec.QueryRowContext(ctx, findUniqueKeyOwnerQuery, key.Type().String(), key.Value()).Scan(&entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return entityID, err == nil, err
}

const findUniqueKeyOwnerWithoutQuery = `
	SELECT u.entity_id
	FROM domain_unique_keys u
	JOIN domain_unique_key_types t ON t.id = u.unique_key_type_id
	WHERE t.name = $1 AND u.key = $2
	  AND NOT EXISTS (
	    SELECT 1 FROM domain_unique_keys w JOIN domain_unique_key_types wt ON wt.id = w.unique_key_type_id
	    WHERE wt.name = $3 AND w.entity_id = u.entity_id
	  )
`

// key を確保している集約のうち、without の種別の key を持たないものを返す（例: 紐づけていないオペレーター）
func findUniqueKeyOwnerWithout(ctx context.Context, key domain.UniqueKey, without domain.UniqueKeyType) (int64, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, false, err
	}
	var entityID int64
	err = exec.QueryRowContext(ctx, findUniqueKeyOwnerWithoutQuery, key.Type().String(), key.Value(), without.String()).Scan(&entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return entityID, err == nil, err
}

const existsOfKeyTypeQuery = `
	SELECT EXISTS (
	  SELECT 1 FROM domain_unique_keys u JOIN domain_unique_key_types t ON t.id = u.unique_key_type_id WHERE t.name = $1
	)
`

func existsOfKeyType(ctx context.Context, keyType domain.UniqueKeyType) (bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	err = exec.QueryRowContext(ctx, existsOfKeyTypeQuery, keyType.String()).Scan(&exists)
	return exists, err
}

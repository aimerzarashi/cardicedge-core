package constraint

import (
	"context"
	"database/sql"
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 上限未満のときだけ 1 増やす、1 つの文。行が無ければ 1 で作り（上限は 1 以上なので、最初の 1 は必ず収まる）、
// 行があれば used が上限未満のときだけ更新する。上限のときは行が返らない。
// 数えてから足す 2 つの文にしないのは、同時に取り合ったとき、どちらも上限未満と読んで上限を超えるため（錠を取る案も、
// 決まりが表ではなく錠の使い方に置かれるため採らなかった）。1 つの文なら、DB が同じ行の更新を順番に行い、後から来たほうは
// 更新後の値で WHERE を評価し直すので、流し直しは要らない。
// 上限の値は DDL に書かず、文に渡す（上限を変えても DDL を変えないため）。種別は名前から引く（種別が無いと行が返らず
// 上限と区別できないが、起動時に domaindef で照合している）
const reserveQuotaQuery = `
	INSERT INTO domain_quotas (quota_type_id, owner_id, used)
	SELECT t.id, $2, 1
	FROM domain_quota_types t
	WHERE t.name = $1
	ON CONFLICT (quota_type_id, owner_id) DO UPDATE SET used = domain_quotas.used + 1
	WHERE domain_quotas.used < $3
	RETURNING used
`

// 上限に達したことは、エラーのログも出さず、トランザクションも中断しない形（行が返らないこと）で知り、limitReached を返す。
// 一意性の確保（updateUniqueKey）と同じく、ふだんの上限で PostgreSQL が ERROR のログを出さないため
func reserveQuota(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit, limitReached error) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	var used int
	err = exec.QueryRowContext(ctx, reserveQuotaQuery, quota.Type().String(), quota.OwnerID(), limit.Int()).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return limitReached
	}
	return err
}

// used が 0 のときは減らさず、エラーにもしない。0 のときに引くと CHECK（0 以上）に反してトランザクションが中断される。
// 0 で解放が呼ばれるのは確保していない数を戻すコードの誤りだけで、それで消す操作ごと止めるより、数が 0 未満にならないことだけを保つ
const releaseQuotaQuery = `
	UPDATE domain_quotas q
	SET used = q.used - 1
	FROM domain_quota_types t
	WHERE t.id = q.quota_type_id AND t.name = $1 AND q.owner_id = $2 AND q.used > 0
`

func releaseQuota(ctx context.Context, quota domain.Quota) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, releaseQuotaQuery, quota.Type().String(), quota.OwnerID())
	return err
}

const countQuotaQuery = `
	SELECT q.used
	FROM domain_quotas q
	JOIN domain_quota_types t ON t.id = q.quota_type_id
	WHERE t.name = $1 AND q.owner_id = $2
`

// 持ち主が今使っている数（domain.CountQuotaFunc）。まだ 1 つも確保していなければ行が無いので 0。
// 種類ごとの関数にしないのは、数えるだけで、上限のエラー（種類ごとに違う）を返さないため
func CountQuota(ctx context.Context, quota domain.Quota) (int, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var used int
	err = exec.QueryRowContext(ctx, countQuotaQuery, quota.Type().String(), quota.OwnerID()).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}

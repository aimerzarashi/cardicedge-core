package eventdriven

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

var ErrSubscriberNotFound = errors.New("eventdriven: subscriber not found")

// event_subscribers.kind の値
const (
	KindProjection = "projection"
	KindPolicy     = "policy"
)

// SubscriberDef は購読者の id と名前。id は自動で振らず、コードの一覧（internal/wiring）で決める（DB ごと・起動の順番で
// 変わりうるため。core#33）。Hub には、その Hub が動くサービスの購読者だけを渡す
type SubscriberDef struct {
	ID   int16
	Kind string
	Name string
}

// 投影の購読者
func ProjectionSubscriber(id int16, name string) SubscriberDef {
	return SubscriberDef{ID: id, Kind: KindProjection, Name: name}
}

// ポリシーの購読者
func PolicySubscriber(id int16, name string) SubscriberDef {
	return SubscriberDef{ID: id, Kind: KindPolicy, Name: name}
}

type subscriberKey struct {
	kind string
	name string
}

// Sync は、Hub に渡した購読者と登録した投影・ポリシーを正として、DB の購読者・購読をそれに合わせる（core#33）。
// 購読者は無ければ足し、DB の行が渡した購読者とちょうど一致しなければ（同じ id で名前が違う・渡していない購読者の行がある）
// エラーを返す（消さない。チェックポイント・やることも消えるため）。購読は、登録した宣言（ProjectionSpec・PolicySpec の Subscriptions）に
// 合わせて足し・消す。投影が足した組には、すでにある集約の分のやることの行を入れる（入れないと、次にイベントが来るまで作られない）。
// ポリシーが足した組には、やることの行ではなく、すでにある集約のチェックポイントを最新の版で入れる（今から先のイベントだけを
// 処理し、入れないと次のイベントで過去の分まで処理してしまう）。
// 消した組のやること・チェックポイント・失敗の記録も消す。起動時に、投影・ポリシーをすべて登録した後・Start の前に、合わせるトランザクションの中で呼ぶ。
// 購読を消すのは、新旧の core が同時に動くと書き換え合いうるが、今は順に入れ替えるデプロイをしていないため（core#19 で考え直す）
func (h *Hub) Sync(ctx context.Context) error {
	if h.started.Load() {
		return errors.New("eventdriven: Sync is called after Start")
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	if err := h.verifyRegistered(); err != nil {
		return err
	}
	for _, def := range h.subscriberDefs {
		if _, err := exec.ExecContext(ctx, `INSERT INTO event_subscribers (id, kind, name) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			def.ID, def.Kind, def.Name); err != nil {
			return fmt.Errorf("eventdriven: failed to add event_subscribers: %w", err)
		}
	}
	if err := h.verifySubscribers(ctx, exec); err != nil {
		return err
	}
	for _, def := range h.subscriberDefs {
		if err := h.syncSubscriptions(ctx, exec, def.ID); err != nil {
			return err
		}
	}
	return nil
}

// 一覧にあるのに組み立てで登録し忘れた購読者は、DB の行だけができて誰も処理しないため、止める
func (h *Hub) verifyRegistered() error {
	var errs []error
	for _, def := range h.subscriberDefs {
		if _, ok := h.registered[def.ID]; !ok {
			errs = append(errs, fmt.Errorf("subscriber is not registered: kind=%s, name=%s", def.Kind, def.Name))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("eventdriven: %w", err)
	}
	return nil
}

// 行を「id: kind/name」の文字列にして、両方向に比べる
func (h *Hub) verifySubscribers(ctx context.Context, exec rdb.Executor) error {
	declared := make([]string, 0, len(h.subscriberDefs))
	for _, def := range h.subscriberDefs {
		declared = append(declared, fmt.Sprintf("%d: %s/%s", def.ID, def.Kind, def.Name))
	}
	rows, err := exec.QueryContext(ctx, `SELECT id || ': ' || kind || '/' || name FROM event_subscribers`)
	if err != nil {
		return fmt.Errorf("eventdriven: failed to read event_subscribers: %w", err)
	}
	defer rows.Close()
	var inDB []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return err
		}
		inDB = append(inDB, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var errs []error
	for _, row := range sortedDiff(declared, inDB) {
		errs = append(errs, fmt.Errorf("subscriber is not in event_subscribers: %s", row))
	}
	for _, row := range sortedDiff(inDB, declared) {
		errs = append(errs, fmt.Errorf("event_subscribers has a subscriber not registered in code: %s", row))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("eventdriven: %w", err)
	}
	return nil
}

func (h *Hub) syncSubscriptions(ctx context.Context, exec rdb.Executor, subscriberID int16) error {
	sub := h.registered[subscriberID]
	declared := sub.entityTypeIDs
	inDB, err := subscriptionsInDB(ctx, exec, subscriberID)
	if err != nil {
		return err
	}
	for _, entityTypeID := range declared {
		if slices.Contains(inDB, entityTypeID) {
			continue
		}
		if _, err := exec.ExecContext(ctx, `INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES ($1, $2)`,
			subscriberID, entityTypeID.Int16()); err != nil {
			return fmt.Errorf("eventdriven: failed to add event_subscriptions: %w", err)
		}
		existingQuery := enqueueExistingQuery
		if sub.kind == KindPolicy {
			existingQuery = checkpointExistingQuery
		}
		if _, err := exec.ExecContext(ctx, existingQuery, subscriberID, entityTypeID.Int16()); err != nil {
			return fmt.Errorf("eventdriven: failed to handle existing aggregates: %w", err)
		}
	}
	for _, entityTypeID := range inDB {
		if slices.Contains(declared, entityTypeID) {
			continue
		}
		for _, query := range removeSubscriptionQueries {
			if _, err := exec.ExecContext(ctx, query, subscriberID, entityTypeID.Int16()); err != nil {
				return fmt.Errorf("eventdriven: failed to remove subscription: %w", err)
			}
		}
	}
	return nil
}

// 購読を足す前に登録された集約も作り直すため、集約ごとに最新の版で、やることの行を入れる（トリガーと同じく、すでにあれば版を上げる）
const enqueueExistingQuery = `
	INSERT INTO event_subscriber_pending (subscriber_id, entity_type_id, entity_id, version)
	SELECT $1, entity_type_id, entity_id, max(aggregate_version)
	FROM domain_events
	WHERE entity_type_id = $2
	GROUP BY entity_type_id, entity_id
	ON CONFLICT (subscriber_id, entity_type_id, entity_id) DO UPDATE
	SET version = GREATEST(event_subscriber_pending.version, EXCLUDED.version)
`

// ポリシーは今から先のイベントだけを処理するため、購読を足す前からある集約は、最新の版まで処理したことにする。
// すでに行があれば（購読を消してから足し直す前に残っていた等）動かさない
const checkpointExistingQuery = `
	INSERT INTO event_subscriber_checkpoints (subscriber_id, entity_type_id, entity_id, last_version, updated_at)
	SELECT $1, entity_type_id, entity_id, max(aggregate_version), now()
	FROM domain_events
	WHERE entity_type_id = $2
	GROUP BY entity_type_id, entity_id
	ON CONFLICT (subscriber_id, entity_type_id, entity_id) DO NOTHING
`

// やること・チェックポイントを残すと、購読をやめた種別の行を Poller が拾い、振り分け先の無い集約を作り直そうとするため、一緒に消す。
// 失敗の記録はポリシーにしか無いが、投影の分は行が無いだけで同じ消し方でよい
var removeSubscriptionQueries = []string{
	`DELETE FROM event_subscriber_failures WHERE subscriber_id = $1 AND entity_type_id = $2`,
	`DELETE FROM event_subscriber_pending WHERE subscriber_id = $1 AND entity_type_id = $2`,
	`DELETE FROM event_subscriber_checkpoints WHERE subscriber_id = $1 AND entity_type_id = $2`,
	`DELETE FROM event_subscriptions WHERE subscriber_id = $1 AND entity_type_id = $2`,
}

func subscriptionsInDB(ctx context.Context, exec rdb.Executor, subscriberID int16) ([]domaindef.EntityTypeID, error) {
	rows, err := exec.QueryContext(ctx, `SELECT entity_type_id FROM event_subscriptions WHERE subscriber_id = $1 ORDER BY entity_type_id`, subscriberID)
	if err != nil {
		return nil, fmt.Errorf("eventdriven: failed to read event_subscriptions: %w", err)
	}
	defer rows.Close()
	var entityTypeIDs []domaindef.EntityTypeID
	for rows.Next() {
		var entityTypeID int16
		if err := rows.Scan(&entityTypeID); err != nil {
			return nil, err
		}
		entityTypeIDs = append(entityTypeIDs, domaindef.NewEntityTypeID(entityTypeID))
	}
	return entityTypeIDs, rows.Err()
}

// a にあって b に無いものを、並べて返す（誤りの並びを毎回同じにするため）
func sortedDiff(a, b []string) []string {
	var diff []string
	for _, value := range a {
		if !slices.Contains(b, value) {
			diff = append(diff, value)
		}
	}
	slices.Sort(diff)
	return diff
}

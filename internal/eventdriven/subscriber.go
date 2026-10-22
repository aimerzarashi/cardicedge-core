package eventdriven

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

var ErrSubscriberNotFound = errors.New("eventdriven: subscriber not found")

// event_subscribers.kind の値
const kindProjection = "projection"

type subscriberKey struct {
	kind string
	name string
}

// event_subscribers の (kind, name) と id の対応と、event_subscriptions（購読する集約の種別）。参照の api はパッケージの変数に持つが、
// domaindef と同じく読み込んだものを渡す（テストが自分の DB で読み込んだものを使えるように）
type Subscribers struct {
	ids           map[subscriberKey]int16
	subscriptions map[int16]map[domaindef.EntityTypeID]bool
}

// アプリケーションの起動時に呼ぶ
func LoadSubscribers(ctx context.Context) (*Subscribers, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := models.EventSubscribers().All(ctx, exec)
	if err != nil {
		return nil, err
	}
	subscribers := &Subscribers{
		ids:           make(map[subscriberKey]int16, len(rows)),
		subscriptions: make(map[int16]map[domaindef.EntityTypeID]bool, len(rows)),
	}
	for _, row := range rows {
		subscribers.ids[subscriberKey{kind: row.Kind, name: row.Name}] = row.ID
	}
	if err := subscribers.loadSubscriptions(ctx, exec); err != nil {
		return nil, err
	}
	return subscribers, nil
}

// sqlboiler は 2 つの表をつなぐだけの表にモデルを作らないため、SQL で読む
func (s *Subscribers) loadSubscriptions(ctx context.Context, exec rdb.Executor) error {
	rows, err := exec.QueryContext(ctx, `SELECT subscriber_id, entity_type_id FROM event_subscriptions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var subscriberID, entityTypeID int16
		if err := rows.Scan(&subscriberID, &entityTypeID); err != nil {
			return err
		}
		if s.subscriptions[subscriberID] == nil {
			s.subscriptions[subscriberID] = make(map[domaindef.EntityTypeID]bool)
		}
		s.subscriptions[subscriberID][domaindef.NewEntityTypeID(entityTypeID)] = true
	}
	return rows.Err()
}

// コードの宣言と event_subscriptions が両方向で一致するかを確かめる。トリガーはこの表で購読者を引くため、
// 食い違うと、やることの行が入らない（作り直しが Poller に拾われない）か、誰も消さない行が残る
func (s *Subscribers) verifySubscriptions(subscriberID int16, declared []domaindef.EntityTypeID) error {
	inDB := s.subscriptions[subscriberID]
	var errs []error
	for _, entityTypeID := range declared {
		if !inDB[entityTypeID] {
			errs = append(errs, fmt.Errorf("subscription is not in event_subscriptions: entity_type_id=%d", entityTypeID.Int16()))
		}
	}
	declaredSet := make(map[domaindef.EntityTypeID]bool, len(declared))
	for _, entityTypeID := range declared {
		declaredSet[entityTypeID] = true
	}
	for entityTypeID := range inDB {
		if !declaredSet[entityTypeID] {
			errs = append(errs, fmt.Errorf("subscription in event_subscriptions is not declared: entity_type_id=%d", entityTypeID.Int16()))
		}
	}
	return errors.Join(errs...)
}

// 誤りの並びを毎回同じにするため、名前の順に返す
func (s *Subscribers) names(kind string) []string {
	var names []string
	for key := range s.ids {
		if key.kind == kind {
			names = append(names, key.name)
		}
	}
	slices.Sort(names)
	return names
}

func (s *Subscribers) id(kind, name string) (int16, error) {
	id, ok := s.ids[subscriberKey{kind: kind, name: name}]
	if !ok {
		return 0, fmt.Errorf("%w: kind=%s, name=%s", ErrSubscriberNotFound, kind, name)
	}
	return id, nil
}

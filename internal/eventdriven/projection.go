package eventdriven

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
)

// read model の状態の型が満たす。ゼロ値を初期状態（まだ投影していない）とし、集約のイベントを順に当てて進める
type ReadModel[S any] interface {
	// 必要なイベントの型だけを扱い、ほかは状態をそのまま返す（複数の集約を購読できるように）
	Evolve(event any) S
	// 集約が削除された。この状態の行は Upsert せず Delete する
	IsTerminal() bool
}

type ReadModelWriteFunc[S any] func(ctx context.Context, state S) error

type ProjectionSpec[S ReadModel[S]] struct {
	// event_subscribers.name（kind は projection）
	Name string
	// 購読する集約の種別。その種別のイベントをすべて受け取る
	Subscriptions []domain.EntityType
	Upsert        ReadModelWriteFunc[S]
	Delete        ReadModelWriteFunc[S]
}

// 宣言・照合に誤りがあれば登録せずにエラーを返す。起動時に呼び、エラーなら起動を止める
func RegisterProjection[S ReadModel[S]](hub *Hub, spec ProjectionSpec[S]) error {
	if err := spec.validate(); err != nil {
		return fmt.Errorf("eventdriven: projection %q: %w", spec.Name, err)
	}
	return hub.register(&registeredSubscriber{kind: KindProjection, name: spec.Name, projection: &specProjection[S]{spec: spec}}, spec.Subscriptions)
}

func (s ProjectionSpec[S]) validate() error {
	var errs []error
	if s.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if len(s.Subscriptions) == 0 {
		errs = append(errs, errors.New("subscriptions is required"))
	}
	if s.Upsert == nil {
		errs = append(errs, errors.New("upsert is required"))
	}
	if s.Delete == nil {
		errs = append(errs, errors.New("delete is required"))
	}
	return errors.Join(errs...)
}

// Hub が扱う、状態の型を消した投影。参照の api は状態を any で Hub とやり取りし、そのたびに型を確かめていたが、
// イベントを当てて書くまでをこの中で済ませ、型 S のまま扱う（型が食い違うという起こりえない場合を作らないため）
type projection interface {
	project(ctx context.Context, events []eventstore.LoadedEvent) error
}

type specProjection[S ReadModel[S]] struct {
	spec ProjectionSpec[S]
}

func (p *specProjection[S]) project(ctx context.Context, events []eventstore.LoadedEvent) error {
	var state S
	for _, event := range events {
		state = state.Evolve(event.Event)
	}
	if state.IsTerminal() {
		return p.spec.Delete(ctx, state)
	}
	return p.spec.Upsert(ctx, state)
}

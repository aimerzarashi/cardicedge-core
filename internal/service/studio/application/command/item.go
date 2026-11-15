package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type ItemHandler struct {
	eventStore domain.EventStore[item.ID, item.Event, item.State]
	identifier domain.Identifier[item.ID]
	updateName item.UpdateNameFunc
	now        func() time.Time
}

func NewItemHandler(
	eventStore domain.EventStore[item.ID, item.Event, item.State],
	identifier domain.Identifier[item.ID],
	updateName item.UpdateNameFunc,
	now func() time.Time,
) *ItemHandler {
	return &ItemHandler{eventStore: eventStore, identifier: identifier, updateName: updateName, now: now}
}

// presentation 層にドメインの型（item.Item・card.Name）を見せないため、表示に要る値だけを返す
type RegisteredItem struct {
	ID           uuid.UUID
	Name         string
	Cost         int
	Rarity       string
	Category     string
	Effects      []Effect
	RegisteredAt time.Time
	UpdatedAt    time.Time
	// 登録した直後の更新の番号。画面が公開する・使用不可にするときに、そのまま渡す
	Revision int64
}

// 操作した人は context のオペレーター（認証ミドルウェアが載せる）。入力の誤りは input.Errors、
// 使われている名前なら ErrCardNameAlreadyInUse
func (c *ItemHandler) Register(ctx context.Context, name string) (RegisteredItem, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return RegisteredItem{}, ErrNotOperator
	}
	cardName, invalid := newCardName("name", name)
	if invalid != nil {
		return RegisteredItem{}, input.Errors{invalid}
	}
	// 結果の日時は Decide に渡す時刻と同じ（イベントの発生時刻。State は登録した日時を持たないため、State から読まない）
	now := c.now()
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (item.Item, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return item.Item{}, err
		}
		if err := c.updateName(ctx, id, cardName); err != nil {
			return item.Item{}, err
		}
		cmd := &item.RegisterCommand{ID: id, PublicID: publicID, Name: cardName, RegisteredBy: registeredBy}
		events, err := item.Decide(item.InitialState(), cmd, now)
		if err != nil {
			return item.Item{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return item.Item{}, err
		}
		state := item.InitialState()
		for _, event := range events {
			if state, err = item.Evolve(state, event); err != nil {
				return item.Item{}, err
			}
		}
		return state.(item.Item), nil
	})
	if errors.Is(err, card.ErrNameAlreadyInUse) {
		return RegisteredItem{}, ErrCardNameAlreadyInUse
	}
	if err != nil {
		return RegisteredItem{}, err
	}
	// 登録した直後は変更が無いため、最後に変更した日時は登録した日時と同じ
	return RegisteredItem{
		ID: registered.PublicID, Name: cardName.String(), Cost: registered.Cost.Int(), Rarity: registered.Rarity.String(),
		Category: registered.Category.String(), Effects: effectsOf(registered.Effects),
		RegisteredAt: now, UpdatedAt: now, Revision: registered.Version,
	}, nil
}

// 公開中にする（準備中・使用不可から）。公開中にもう一度公開しても変わらず、revision も進まない。revision は画面で見たカードの
// 更新の番号。無ければ ErrCardNotFound、画面を開いた後にほかの操作で変わっていれば ErrRevisionConflict
func (c *ItemHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.change(ctx, id, revision, &item.PublishCommand{PublishedBy: publishedBy})
}

// 使用不可にする（公開中から）。使用不可にもう一度しても変わらない。準備中からは ErrInvalidCardStatusTransition。
// ほかのエラーは Publish と同じ
func (c *ItemHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.change(ctx, id, revision, &item.RetireCommand{RetiredBy: retiredBy})
}

// コストを変える（準備中だけ）。同じ値なら変わらず、revision も進まない。0 未満は input.Errors、準備中でなければ ErrCardNotDraft。
// ほかのエラーは Publish と同じ
func (c *ItemHandler) ChangeCost(ctx context.Context, id uuid.UUID, revision int64, cost int) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newCardCost("cost", cost)
	if invalid != nil {
		return ChangedCard{}, input.Errors{invalid}
	}
	return c.change(ctx, id, revision, &item.ChangeCostCommand{Cost: value, ChangedBy: changedBy})
}

// 希少度を変える（準備中だけ）。エラーは ChangeCost と同じ
func (c *ItemHandler) ChangeRarity(ctx context.Context, id uuid.UUID, revision int64, rarity string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newCardRarity("rarity", rarity)
	if invalid != nil {
		return ChangedCard{}, input.Errors{invalid}
	}
	return c.change(ctx, id, revision, &item.ChangeRarityCommand{Rarity: value, ChangedBy: changedBy})
}

// 分類を変える（準備中だけ）。同じ値なら変わらず、revision も進まない。知らない値は input.Errors（InvalidFormat）。アイテムでない ID は ErrCardNotFound。
// 今の効果を持てない分類は ErrItemEffectNotAllowed。ほかのエラーは ChangeCost と同じ。分類はアイテムだけが持つので、CardHandler を通さずここで受ける
func (c *ItemHandler) ChangeCategory(ctx context.Context, id uuid.UUID, revision int64, category string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, err := card.NewItemCategory(category)
	if err != nil {
		return ChangedCard{}, input.Errors{&input.InvalidFormat{Field: "category"}}
	}
	return c.change(ctx, id, revision, &item.ChangeCategoryCommand{Category: value, ChangedBy: changedBy})
}

// 効果の並びを変える（準備中だけ。変えた後の並びの全体）。同じ並びなら変わらず、revision も進まない。種類と値の組み合わせが合わない・
// 値の範囲の誤りは input.Errors（項目は effects.N・effects.N.value・effects.N.minimum・effects.N.maximum・effects.N.percent・effects.N.constraints.M・
// effects.N.constraints.M.elements）、今の分類で持てない効果は ErrItemEffectNotAllowed。ほかのエラーは ChangeCategory と同じ
func (c *ItemHandler) ChangeEffects(ctx context.Context, id uuid.UUID, revision int64, effects []Effect) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newEffects(effects)
	if len(invalid) > 0 {
		return ChangedCard{}, invalid
	}
	return c.change(ctx, id, revision, &item.ChangeEffectsCommand{Effects: value, ChangedBy: changedBy})
}

func (c *ItemHandler) change(ctx context.Context, publicID uuid.UUID, revision int64, cmd item.Command) (ChangedCard, error) {
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedCard, error) {
		loaded, err := c.load(ctx, publicID, revision)
		if err != nil {
			return ChangedCard{}, err
		}
		events, err := item.Decide(loaded, cmd, c.now())
		if err != nil {
			return ChangedCard{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedCard{}, err
		}
		var state item.State = loaded
		for _, event := range events {
			if state, err = item.Evolve(state, event); err != nil {
				return ChangedCard{}, err
			}
		}
		return ChangedCard{ID: publicID, Revision: state.(item.Item).Version}, nil
	})
	if errors.Is(err, item.ErrEffectNotAllowed) {
		return ChangedCard{}, ErrItemEffectNotAllowed
	}
	return changed, cardChangeError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *ItemHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (item.Item, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return item.Item{}, ErrCardNotFound
	}
	if err != nil {
		return item.Item{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return item.Item{}, err
	}
	loaded, ok := state.(item.Item)
	if !ok {
		return item.Item{}, ErrCardNotFound
	}
	if loaded.Version != revision {
		return item.Item{}, ErrRevisionConflict
	}
	return loaded, nil
}

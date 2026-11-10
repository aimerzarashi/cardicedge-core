package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type SpellHandler struct {
	eventStore domain.EventStore[spell.ID, spell.Event, spell.State]
	identifier domain.Identifier[spell.ID]
	updateName spell.UpdateNameFunc
	now        func() time.Time
}

func NewSpellHandler(
	eventStore domain.EventStore[spell.ID, spell.Event, spell.State],
	identifier domain.Identifier[spell.ID],
	updateName spell.UpdateNameFunc,
	now func() time.Time,
) *SpellHandler {
	return &SpellHandler{eventStore: eventStore, identifier: identifier, updateName: updateName, now: now}
}

// presentation 層にドメインの型（spell.Spell・card.Name）を見せないため、表示に要る値だけを返す
type RegisteredSpell struct {
	ID     uuid.UUID
	Name   string
	Cost   int
	Rarity string
	// 登録の直後は効果なし
	Effect       SpellEffect
	RegisteredAt time.Time
	UpdatedAt    time.Time
	// 登録した直後の更新の番号。画面が公開する・使用不可にするときに、そのまま渡す
	Revision int64
}

// 操作した人は context のオペレーター（認証ミドルウェアが載せる）。入力の誤りは input.Errors、
// 使われている名前なら ErrCardNameAlreadyInUse
func (c *SpellHandler) Register(ctx context.Context, name string) (RegisteredSpell, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return RegisteredSpell{}, ErrNotOperator
	}
	cardName, invalid := newCardName("name", name)
	if invalid != nil {
		return RegisteredSpell{}, input.Errors{invalid}
	}
	// 結果の日時は Decide に渡す時刻と同じ（イベントの発生時刻。State は登録した日時を持たないため、State から読まない）
	now := c.now()
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (spell.Spell, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return spell.Spell{}, err
		}
		if err := c.updateName(ctx, id, cardName); err != nil {
			return spell.Spell{}, err
		}
		cmd := &spell.RegisterCommand{ID: id, PublicID: publicID, Name: cardName, RegisteredBy: registeredBy}
		events, err := spell.Decide(spell.InitialState(), cmd, now)
		if err != nil {
			return spell.Spell{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return spell.Spell{}, err
		}
		state := spell.InitialState()
		for _, event := range events {
			if state, err = spell.Evolve(state, event); err != nil {
				return spell.Spell{}, err
			}
		}
		return state.(spell.Spell), nil
	})
	if errors.Is(err, card.ErrNameAlreadyInUse) {
		return RegisteredSpell{}, ErrCardNameAlreadyInUse
	}
	if err != nil {
		return RegisteredSpell{}, err
	}
	// 登録した直後は変更が無いため、最後に変更した日時は登録した日時と同じ
	return RegisteredSpell{
		ID: registered.PublicID, Name: cardName.String(), Cost: registered.Cost.Int(), Rarity: registered.Rarity.String(),
		RegisteredAt: now, UpdatedAt: now, Revision: registered.Version,
	}, nil
}

// 公開中にする（準備中・使用不可から）。公開中にもう一度公開しても変わらず、revision も進まない。revision は画面で見たカードの
// 更新の番号。無ければ ErrCardNotFound、画面を開いた後にほかの操作で変わっていれば ErrRevisionConflict
func (c *SpellHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.change(ctx, id, revision, &spell.PublishCommand{PublishedBy: publishedBy})
}

// 使用不可にする（公開中から）。使用不可にもう一度しても変わらない。準備中からは ErrInvalidCardStatusTransition。
// ほかのエラーは Publish と同じ
func (c *SpellHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.change(ctx, id, revision, &spell.RetireCommand{RetiredBy: retiredBy})
}

// コストを変える（準備中だけ）。同じ値なら変わらず、revision も進まない。0 未満は input.Errors、準備中でなければ ErrCardNotDraft。
// ほかのエラーは Publish と同じ
func (c *SpellHandler) ChangeCost(ctx context.Context, id uuid.UUID, revision int64, cost int) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newCardCost("cost", cost)
	if invalid != nil {
		return ChangedCard{}, input.Errors{invalid}
	}
	return c.change(ctx, id, revision, &spell.ChangeCostCommand{Cost: value, ChangedBy: changedBy})
}

// 希少度を変える（準備中だけ）。エラーは ChangeCost と同じ
func (c *SpellHandler) ChangeRarity(ctx context.Context, id uuid.UUID, revision int64, rarity string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newCardRarity("rarity", rarity)
	if invalid != nil {
		return ChangedCard{}, input.Errors{invalid}
	}
	return c.change(ctx, id, revision, &spell.ChangeRarityCommand{Rarity: value, ChangedBy: changedBy})
}

// スペルの効果の入力・結果。種類ごとの値のどれか 1 つだけを持ち、どれも無ければ効果なし（domain の spell.Effect と同じ形）
type SpellEffect struct {
	DrawCards *SpellDrawCards
	GainMagic *SpellGainMagic
	RollDice  *SpellRollDice
}

type SpellDrawCards struct {
	Count int
}

type SpellGainMagic struct {
	PerLap int
}

type SpellRollDice struct {
	Count int
}

// 効果を変える（準備中だけ）。変えた後の効果の全体を受け取り、どの種類も無ければ効果なしにする。同じ値なら変わらず、revision も進まない。
// 値が下限より小さいのは input.Errors（項目は effect.drawCards.count・effect.gainMagic.perLap・effect.rollDice.count）。
// 種類が 2 つ以上あるのはコードの誤り（presentation 層が 1 つだけを渡す）。ほかのエラーは ChangeCost と同じ
func (c *SpellHandler) ChangeEffect(ctx context.Context, id uuid.UUID, revision int64, effect SpellEffect) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, err := newSpellEffect(effect)
	if err != nil {
		return ChangedCard{}, err
	}
	return c.change(ctx, id, revision, &spell.ChangeEffectCommand{Effect: value, ChangedBy: changedBy})
}

func newSpellEffect(effect SpellEffect) (spell.Effect, error) {
	switch {
	case effect.DrawCards != nil:
		count, err := spell.NewDrawCount(effect.DrawCards.Count)
		if err != nil {
			return spell.Effect{}, input.Errors{&input.TooSmall{Field: "effect.drawCards.count", Min: 1}}
		}
		return spell.NewDrawCardsEffect(count), nil
	case effect.GainMagic != nil:
		perLap, err := spell.NewMagicPerLap(effect.GainMagic.PerLap)
		if err != nil {
			return spell.Effect{}, input.Errors{&input.TooSmall{Field: "effect.gainMagic.perLap", Min: 1}}
		}
		return spell.NewGainMagicEffect(perLap), nil
	case effect.RollDice != nil:
		count, err := spell.NewDiceCount(effect.RollDice.Count)
		if err != nil {
			return spell.Effect{}, input.Errors{&input.TooSmall{Field: "effect.rollDice.count", Min: 2}}
		}
		return spell.NewRollDiceEffect(count), nil
	default:
		return spell.NoEffect(), nil
	}
}

func (c *SpellHandler) change(ctx context.Context, publicID uuid.UUID, revision int64, cmd spell.Command) (ChangedCard, error) {
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedCard, error) {
		loaded, err := c.load(ctx, publicID, revision)
		if err != nil {
			return ChangedCard{}, err
		}
		events, err := spell.Decide(loaded, cmd, c.now())
		if err != nil {
			return ChangedCard{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedCard{}, err
		}
		var state spell.State = loaded
		for _, event := range events {
			if state, err = spell.Evolve(state, event); err != nil {
				return ChangedCard{}, err
			}
		}
		return ChangedCard{ID: publicID, Revision: state.(spell.Spell).Version}, nil
	})
	return changed, cardChangeError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *SpellHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (spell.Spell, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return spell.Spell{}, ErrCardNotFound
	}
	if err != nil {
		return spell.Spell{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return spell.Spell{}, err
	}
	loaded, ok := state.(spell.Spell)
	if !ok {
		return spell.Spell{}, ErrCardNotFound
	}
	if loaded.Version != revision {
		return spell.Spell{}, ErrRevisionConflict
	}
	return loaded, nil
}

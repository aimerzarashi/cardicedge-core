package command

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

// Creature・Item・Spell の登録で共有する（カード名は 3 種類をまたいで一意）。入力の誤りでなければ input.Error は nil
func newCardName(field, value string) (card.Name, input.Error) {
	name, err := card.NewName(value)
	switch {
	case errors.Is(err, card.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, card.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: card.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}

// Creature・Item・Spell の性能の変更で共有する
func newCardCost(field string, value int) (card.Cost, input.Error) {
	cost, err := card.NewCost(value)
	if err != nil {
		return 0, &input.TooSmall{Field: field, Min: 0}
	}
	return cost, nil
}

// 値は GraphQL の enum から写すので、知らない値はコードの誤り。入力の形の誤りとして返す
func newCardRarity(field, value string) (card.Rarity, input.Error) {
	rarity, err := card.NewRarity(value)
	if err != nil {
		return "", &input.InvalidFormat{Field: field}
	}
	return rarity, nil
}

// 種類ごとのハンドラーの、状態・性能を変える操作のドメインのエラーを application 層のエラーにする
func cardChangeError(err error) error {
	switch {
	case errors.Is(err, card.ErrInvalidStatusTransition):
		return ErrInvalidCardStatusTransition
	case errors.Is(err, card.ErrNotDraft):
		return ErrCardNotDraft
	default:
		return revisionConflict(err)
	}
}

// 状態を変える操作の結果は、公開 ID と新しい revision だけにする（内容は返さない。PresetDeckHandler の ChangedPresetDeck と同じ理由）
type ChangedCard struct {
	ID       uuid.UUID
	Revision int64
}

// カードの種類ごとのハンドラーが持つ、3 種類に共通の操作（CardHandler が種類で振り分ける先）
type cardChanger interface {
	Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error)
	ChangeCost(ctx context.Context, id uuid.UUID, revision int64, cost int) (ChangedCard, error)
	ChangeRarity(ctx context.Context, id uuid.UUID, revision int64, rarity string) (ChangedCard, error)
}

// 公開 ID からカードの種類を引き、Creature・Item・Spell のハンドラーに振り分ける。カードという集約は作らない
// （名前の一意性だけを種類をまたいで共有する決まり）ため、状態と共通の性能（コスト・希少度）の変更は種類ごとの集約が持ち、
// ここは振り分けだけをする
type CardHandler struct {
	entityRefs domain.EntityRefResolver
	creatures  cardChanger
	items      cardChanger
	spells     cardChanger
}

func NewCardHandler(entityRefs domain.EntityRefResolver, creatures *CreatureHandler, items *ItemHandler, spells *SpellHandler) *CardHandler {
	return &CardHandler{entityRefs: entityRefs, creatures: creatures, items: items, spells: spells}
}

// 公開中にする。カードが無い・カードでない公開 ID は ErrCardNotFound。ほかのエラーは CreatureHandler.Publish と同じ
func (c *CardHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	changer, err := c.changer(ctx, id)
	if err != nil {
		return ChangedCard{}, err
	}
	return changer.Publish(ctx, id, revision)
}

// 使用不可にする。エラーは Publish と CreatureHandler.Retire と同じ
func (c *CardHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	changer, err := c.changer(ctx, id)
	if err != nil {
		return ChangedCard{}, err
	}
	return changer.Retire(ctx, id, revision)
}

// コストを変える。エラーは Publish と CreatureHandler.ChangeCost と同じ
func (c *CardHandler) ChangeCost(ctx context.Context, id uuid.UUID, revision int64, cost int) (ChangedCard, error) {
	changer, err := c.changer(ctx, id)
	if err != nil {
		return ChangedCard{}, err
	}
	return changer.ChangeCost(ctx, id, revision, cost)
}

// 希少度を変える。エラーは Publish と CreatureHandler.ChangeRarity と同じ
func (c *CardHandler) ChangeRarity(ctx context.Context, id uuid.UUID, revision int64, rarity string) (ChangedCard, error) {
	changer, err := c.changer(ctx, id)
	if err != nil {
		return ChangedCard{}, err
	}
	return changer.ChangeRarity(ctx, id, revision, rarity)
}

// 操作した人がいないときは、カードの有無を答える前に断る（種類ごとのハンドラーに届かないため、ここでも確かめる）
func (c *CardHandler) changer(ctx context.Context, id uuid.UUID) (cardChanger, error) {
	if _, ok := authentication.Operator(ctx); !ok {
		return nil, ErrNotOperator
	}
	ref, err := c.entityRefs.Resolve(ctx, id)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return nil, ErrCardNotFound
	}
	if err != nil {
		return nil, err
	}
	switch ref.EntityType() {
	case creature.Types().EntityType:
		return c.creatures, nil
	case item.Types().EntityType:
		return c.items, nil
	case spell.Types().EntityType:
		return c.spells, nil
	default:
		return nil, ErrCardNotFound
	}
}

// プリセットデッキのカード（種類と内部 ID の組）の状態を返す。無い（まだ登録していない）カードは結果に含めない
type CardStatusesFunc func(ctx context.Context, refs []card.Ref) (map[card.Ref]card.Status, error)

// カードの状態を、集約をイベントストアで読んで確かめる。read model（cards）を読まないのは、投影が遅れている間に
// 古い状態で判断しないため。読むだけで書かない
type CardStatusReader struct {
	creatures domain.EventStore[creature.ID, creature.Event, creature.State]
	items     domain.EventStore[item.ID, item.Event, item.State]
	spells    domain.EventStore[spell.ID, spell.Event, spell.State]
}

func NewCardStatusReader(
	creatures domain.EventStore[creature.ID, creature.Event, creature.State],
	items domain.EventStore[item.ID, item.Event, item.State],
	spells domain.EventStore[spell.ID, spell.Event, spell.State],
) *CardStatusReader {
	return &CardStatusReader{creatures: creatures, items: items, spells: spells}
}

func (r *CardStatusReader) Statuses(ctx context.Context, refs []card.Ref) (map[card.Ref]card.Status, error) {
	statuses := make(map[card.Ref]card.Status, len(refs))
	for _, ref := range refs {
		status, found, err := r.status(ctx, ref)
		if err != nil {
			return nil, err
		}
		if found {
			statuses[ref] = status
		}
	}
	return statuses, nil
}

func (r *CardStatusReader) status(ctx context.Context, ref card.Ref) (card.Status, bool, error) {
	switch ref.EntityType {
	case creature.Types().EntityType:
		id, err := creature.NewID(ref.ID)
		if err != nil {
			return "", false, err
		}
		state, err := r.creatures.Load(ctx, id)
		if err != nil {
			return "", false, err
		}
		loaded, ok := state.(creature.Creature)
		return loaded.Status, ok, nil
	case item.Types().EntityType:
		id, err := item.NewID(ref.ID)
		if err != nil {
			return "", false, err
		}
		state, err := r.items.Load(ctx, id)
		if err != nil {
			return "", false, err
		}
		loaded, ok := state.(item.Item)
		return loaded.Status, ok, nil
	case spell.Types().EntityType:
		id, err := spell.NewID(ref.ID)
		if err != nil {
			return "", false, err
		}
		state, err := r.spells.Load(ctx, id)
		if err != nil {
			return "", false, err
		}
		loaded, ok := state.(spell.Spell)
		return loaded.Status, ok, nil
	default:
		return "", false, card.ErrNotCard
	}
}

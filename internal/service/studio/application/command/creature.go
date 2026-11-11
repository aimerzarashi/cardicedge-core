package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type CreatureHandler struct {
	eventStore domain.EventStore[creature.ID, creature.Event, creature.State]
	identifier domain.Identifier[creature.ID]
	updateName creature.UpdateNameFunc
	now        func() time.Time
}

func NewCreatureHandler(
	eventStore domain.EventStore[creature.ID, creature.Event, creature.State],
	identifier domain.Identifier[creature.ID],
	updateName creature.UpdateNameFunc,
	now func() time.Time,
) *CreatureHandler {
	return &CreatureHandler{eventStore: eventStore, identifier: identifier, updateName: updateName, now: now}
}

// presentation 層にドメインの型（creature.Creature・card.Name）を見せないため、表示に要る値だけを返す
type RegisteredCreature struct {
	ID                    uuid.UUID
	Name                  string
	Cost                  int
	Rarity                string
	Elements              []string
	Attack                int
	HitPoints             int
	ItemRestrictions      []string
	PlacementRestrictions []string
	Abilities             []CreatureAbility
	RegisteredAt          time.Time
	UpdatedAt             time.Time
	// 登録した直後の更新の番号。画面が公開する・使用不可にするときに、そのまま渡す
	Revision int64
}

// 操作した人は context のオペレーター（認証ミドルウェアが載せる）。入力の誤りは input.Errors、
// 使われている名前なら ErrCardNameAlreadyInUse
func (c *CreatureHandler) Register(ctx context.Context, name string) (RegisteredCreature, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return RegisteredCreature{}, ErrNotOperator
	}
	cardName, invalid := newCardName("name", name)
	if invalid != nil {
		return RegisteredCreature{}, input.Errors{invalid}
	}
	// 結果の日時は Decide に渡す時刻と同じ（イベントの発生時刻。State は登録した日時を持たないため、State から読まない）
	now := c.now()
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (creature.Creature, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return creature.Creature{}, err
		}
		if err := c.updateName(ctx, id, cardName); err != nil {
			return creature.Creature{}, err
		}
		cmd := &creature.RegisterCommand{ID: id, PublicID: publicID, Name: cardName, RegisteredBy: registeredBy}
		events, err := creature.Decide(creature.InitialState(), cmd, now)
		if err != nil {
			return creature.Creature{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return creature.Creature{}, err
		}
		state := creature.InitialState()
		for _, event := range events {
			if state, err = creature.Evolve(state, event); err != nil {
				return creature.Creature{}, err
			}
		}
		return state.(creature.Creature), nil
	})
	if errors.Is(err, card.ErrNameAlreadyInUse) {
		return RegisteredCreature{}, ErrCardNameAlreadyInUse
	}
	if err != nil {
		return RegisteredCreature{}, err
	}
	// 登録した直後は変更が無いため、最後に変更した日時は登録した日時と同じ
	return RegisteredCreature{
		ID: registered.PublicID, Name: cardName.String(), Cost: registered.Cost.Int(), Rarity: registered.Rarity.String(),
		Elements: registered.Elements.Strings(), Attack: registered.Attack.Int(), HitPoints: registered.HitPoints.Int(), ItemRestrictions: registered.ItemRestrictions.Strings(), PlacementRestrictions: registered.PlacementRestrictions.Strings(), Abilities: creatureAbilities(registered.Abilities),
		RegisteredAt: now, UpdatedAt: now, Revision: registered.Version,
	}, nil
}

// 公開中にする（準備中・使用不可から）。公開中にもう一度公開しても変わらず、revision も進まない。revision は画面で見たカードの
// 更新の番号。無ければ ErrCardNotFound、画面を開いた後にほかの操作で変わっていれば ErrRevisionConflict
func (c *CreatureHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.change(ctx, id, revision, &creature.PublishCommand{PublishedBy: publishedBy})
}

// 使用不可にする（公開中から）。使用不可にもう一度しても変わらない。準備中からは ErrInvalidCardStatusTransition。
// ほかのエラーは Publish と同じ
func (c *CreatureHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.change(ctx, id, revision, &creature.RetireCommand{RetiredBy: retiredBy})
}

// コストを変える（準備中だけ）。同じ値なら変わらず、revision も進まない。0 未満は input.Errors、準備中でなければ ErrCardNotDraft。
// ほかのエラーは Publish と同じ
func (c *CreatureHandler) ChangeCost(ctx context.Context, id uuid.UUID, revision int64, cost int) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newCardCost("cost", cost)
	if invalid != nil {
		return ChangedCard{}, input.Errors{invalid}
	}
	return c.change(ctx, id, revision, &creature.ChangeCostCommand{Cost: value, ChangedBy: changedBy})
}

// 希少度を変える（準備中だけ）。エラーは ChangeCost と同じ
func (c *CreatureHandler) ChangeRarity(ctx context.Context, id uuid.UUID, revision int64, rarity string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, invalid := newCardRarity("rarity", rarity)
	if invalid != nil {
		return ChangedCard{}, input.Errors{invalid}
	}
	return c.change(ctx, id, revision, &creature.ChangeRarityCommand{Rarity: value, ChangedBy: changedBy})
}

// 属性を変える（準備中だけ）。同じ集合なら変わらず、revision も進まない。属性は 1 つ以上（無いときは Required）で、
// 知らない値・無と他の属性の組み合わせ・重複は InvalidFormat。ほかのエラーは ChangeCost と同じ。
// 属性はクリーチャーだけが持つので、CardHandler を通さずここで受ける
func (c *CreatureHandler) ChangeElements(ctx context.Context, id uuid.UUID, revision int64, elements []string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, err := creature.NewElements(elements)
	switch {
	case errors.Is(err, creature.ErrElementsEmpty):
		return ChangedCard{}, input.Errors{&input.Required{Field: "elements"}}
	case err != nil:
		return ChangedCard{}, input.Errors{&input.InvalidFormat{Field: "elements"}}
	}
	return c.change(ctx, id, revision, &creature.ChangeElementsCommand{Elements: value, ChangedBy: changedBy})
}

// 攻撃力と体力を変える（準備中だけ）。どちらも同じ値なら変わらず、revision も進まない。攻撃力は 0 未満、体力は 1 未満が input.Errors
// （両方が誤りならどちらも返す）。ほかのエラーは ChangeElements と同じ
func (c *CreatureHandler) ChangeStats(ctx context.Context, id uuid.UUID, revision int64, attack, hitPoints int) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	invalid := input.Errors{}
	attackValue, err := creature.NewAttack(attack)
	if err != nil {
		invalid = append(invalid, &input.TooSmall{Field: "attack", Min: 0})
	}
	hitPointsValue, err := creature.NewHitPoints(hitPoints)
	if err != nil {
		invalid = append(invalid, &input.TooSmall{Field: "hitPoints", Min: 1})
	}
	if len(invalid) > 0 {
		return ChangedCard{}, invalid
	}
	return c.change(ctx, id, revision, &creature.ChangeStatsCommand{Attack: attackValue, HitPoints: hitPointsValue, ChangedBy: changedBy})
}

// 配置制限を変える（準備中だけ）。同じ集合なら変わらず、revision も進まない。知らない属性・重複は input.Errors。ほかのエラーは ChangeElements と同じ
func (c *CreatureHandler) ChangePlacementRestrictions(ctx context.Context, id uuid.UUID, revision int64, placementRestrictions []string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, err := creature.NewPlacementRestrictions(placementRestrictions)
	if err != nil {
		return ChangedCard{}, input.Errors{&input.InvalidFormat{Field: "placementRestrictions"}}
	}
	return c.change(ctx, id, revision, &creature.ChangePlacementRestrictionsCommand{PlacementRestrictions: value, ChangedBy: changedBy})
}

// アイテム制限を変える（準備中だけ）。同じ集合なら変わらず、revision も進まない。知らない分類・重複は input.Errors。ほかのエラーは ChangeElements と同じ
func (c *CreatureHandler) ChangeItemRestrictions(ctx context.Context, id uuid.UUID, revision int64, itemRestrictions []string) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	value, err := creature.NewItemRestrictions(itemRestrictions)
	if err != nil {
		return ChangedCard{}, input.Errors{&input.InvalidFormat{Field: "itemRestrictions"}}
	}
	return c.change(ctx, id, revision, &creature.ChangeItemRestrictionsCommand{ItemRestrictions: value, ChangedBy: changedBy})
}

// 能力を変える（準備中だけ）。同じ並びなら変わらず、revision も進まない。種類と値の組み合わせの誤り・値の範囲・重複・先制と後手の同時の指定は input.Errors
// （項目は abilities.N・abilities.N.percent・abilities）。ほかのエラーは ChangeElements と同じ
func (c *CreatureHandler) ChangeAbilities(ctx context.Context, id uuid.UUID, revision int64, abilities []CreatureAbility) (ChangedCard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	values := make([]creature.Ability, 0, len(abilities))
	invalid := input.Errors{}
	for i, ability := range abilities {
		value, err := ability.domain()
		switch {
		case errors.Is(err, creature.ErrTollMultiplierPercentTooSmall):
			invalid = append(invalid, &input.TooSmall{Field: fmt.Sprintf("abilities.%d.percent", i), Min: 1})
		case err != nil:
			invalid = append(invalid, &input.InvalidFormat{Field: fmt.Sprintf("abilities.%d", i)})
		}
		values = append(values, value)
	}
	if len(invalid) > 0 {
		return ChangedCard{}, invalid
	}
	value, err := creature.NewAbilities(values)
	if err != nil {
		return ChangedCard{}, input.Errors{&input.InvalidFormat{Field: "abilities"}}
	}
	return c.change(ctx, id, revision, &creature.ChangeAbilitiesCommand{Abilities: value, ChangedBy: changedBy})
}

func (c *CreatureHandler) change(ctx context.Context, publicID uuid.UUID, revision int64, cmd creature.Command) (ChangedCard, error) {
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedCard, error) {
		loaded, err := c.load(ctx, publicID, revision)
		if err != nil {
			return ChangedCard{}, err
		}
		events, err := creature.Decide(loaded, cmd, c.now())
		if err != nil {
			return ChangedCard{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedCard{}, err
		}
		var state creature.State = loaded
		for _, event := range events {
			if state, err = creature.Evolve(state, event); err != nil {
				return ChangedCard{}, err
			}
		}
		return ChangedCard{ID: publicID, Revision: state.(creature.Creature).Version}, nil
	})
	return changed, cardChangeError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *CreatureHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (creature.Creature, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return creature.Creature{}, ErrCardNotFound
	}
	if err != nil {
		return creature.Creature{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return creature.Creature{}, err
	}
	loaded, ok := state.(creature.Creature)
	if !ok {
		return creature.Creature{}, ErrCardNotFound
	}
	if loaded.Version != revision {
		return creature.Creature{}, ErrRevisionConflict
	}
	return loaded, nil
}

// クリーチャーの能力の種類（domain の種類の名前と同じ）
const (
	CreatureAbilityDefensive      = "defensive"
	CreatureAbilityTollMultiplier = "toll_multiplier"
	CreatureAbilityFirstStrike    = "first_strike"
	CreatureAbilityLastStrike     = "last_strike"
)

// クリーチャーの能力の入力と出力。種類と、種類に要る値（通行料の倍率は Percent）
type CreatureAbility struct {
	Kind    string
	Percent *int
}

var errCreatureAbilityShape = errors.New("command: creature ability kind and values do not match")

// 種類と値の組み合わせが合わないものは errCreatureAbilityShape、値の範囲の誤りは domain の誤り
func (a CreatureAbility) domain() (creature.Ability, error) {
	switch {
	case a.Kind == CreatureAbilityDefensive && a.Percent == nil:
		return creature.NewDefensiveAbility(), nil
	case a.Kind == CreatureAbilityTollMultiplier && a.Percent != nil:
		percent, err := creature.NewTollMultiplierPercent(*a.Percent)
		if err != nil {
			return creature.Ability{}, err
		}
		return creature.NewTollMultiplierAbility(percent), nil
	case a.Kind == CreatureAbilityFirstStrike && a.Percent == nil:
		return creature.NewFirstStrikeAbility(), nil
	case a.Kind == CreatureAbilityLastStrike && a.Percent == nil:
		return creature.NewLastStrikeAbility(), nil
	}
	return creature.Ability{}, errCreatureAbilityShape
}

func creatureAbilities(abilities creature.Abilities) []CreatureAbility {
	values := make([]CreatureAbility, len(abilities))
	for i, ability := range abilities {
		switch {
		case ability.IsDefensive():
			values[i] = CreatureAbility{Kind: CreatureAbilityDefensive}
		case ability.TollMultiplier != nil:
			percent := ability.TollMultiplier.Percent.Int()
			values[i] = CreatureAbility{Kind: CreatureAbilityTollMultiplier, Percent: &percent}
		case ability.FirstStrike != nil:
			values[i] = CreatureAbility{Kind: CreatureAbilityFirstStrike}
		case ability.LastStrike != nil:
			values[i] = CreatureAbility{Kind: CreatureAbilityLastStrike}
		}
	}
	return values
}

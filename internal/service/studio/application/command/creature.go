package command

import (
	"context"
	"errors"
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
	ID           uuid.UUID
	Name         string
	RegisteredAt time.Time
	UpdatedAt    time.Time
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
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (creature.Creature, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return creature.Creature{}, err
		}
		if err := c.updateName(ctx, id, cardName); err != nil {
			return creature.Creature{}, err
		}
		cmd := &creature.RegisterCommand{ID: id, PublicID: publicID, Name: cardName, RegisteredBy: registeredBy}
		events, err := creature.Decide(creature.InitialState(), cmd, c.now())
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
		ID: registered.PublicID, Name: registered.Name.String(),
		RegisteredAt: registered.RegisteredAt, UpdatedAt: registered.RegisteredAt, Revision: registered.Version,
	}, nil
}

// 公開中にする（準備中・使用不可から）。公開中にもう一度公開しても変わらず、revision も進まない。revision は画面で見たカードの
// 更新の番号。無ければ ErrCardNotFound、画面を開いた後にほかの操作で変わっていれば ErrRevisionConflict
func (c *CreatureHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.changeStatus(ctx, id, revision, &creature.PublishCommand{PublishedBy: publishedBy})
}

// 使用不可にする（公開中から）。使用不可にもう一度しても変わらない。準備中からは ErrInvalidCardStatusTransition。
// ほかのエラーは Publish と同じ
func (c *CreatureHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedCard, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedCard{}, ErrNotOperator
	}
	return c.changeStatus(ctx, id, revision, &creature.RetireCommand{RetiredBy: retiredBy})
}

func (c *CreatureHandler) changeStatus(ctx context.Context, publicID uuid.UUID, revision int64, cmd creature.Command) (ChangedCard, error) {
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
	if errors.Is(err, card.ErrInvalidStatusTransition) {
		return ChangedCard{}, ErrInvalidCardStatusTransition
	}
	return changed, revisionConflict(err)
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

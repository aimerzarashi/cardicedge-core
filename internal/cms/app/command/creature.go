package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type CreatureCommands struct {
	eventStore domain.EventStore[creature.ID, creature.Event, creature.State]
	identifier domain.Identifier[creature.ID]
	updateName creature.UpdateNameFunc
	now        func() time.Time
}

func NewCreatureCommands(
	eventStore domain.EventStore[creature.ID, creature.Event, creature.State],
	identifier domain.Identifier[creature.ID],
	updateName creature.UpdateNameFunc,
	now func() time.Time,
) *CreatureCommands {
	return &CreatureCommands{eventStore: eventStore, identifier: identifier, updateName: updateName, now: now}
}

// uif にドメインの型（creature.Creature・card.Name）を見せないため、表示に要る値だけを返す
type RegisteredCreature struct {
	ID   uuid.UUID
	Name string
}

// 操作した人は context のオペレーター（認証ミドルウェアが載せる）。入力の誤りは InputErrors、
// 使われている名前なら ErrCardNameAlreadyInUse
func (c *CreatureCommands) Register(ctx context.Context, name string) (RegisteredCreature, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return RegisteredCreature{}, ErrNotOperator
	}
	cardName, invalid := newCardName("name", name)
	if invalid != nil {
		return RegisteredCreature{}, InputErrors{invalid}
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
	return RegisteredCreature{ID: registered.PublicID, Name: registered.Name.String()}, nil
}

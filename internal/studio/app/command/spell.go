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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
)

type SpellCommands struct {
	eventStore domain.EventStore[spell.ID, spell.Event, spell.State]
	identifier domain.Identifier[spell.ID]
	updateName spell.UpdateNameFunc
	now        func() time.Time
}

func NewSpellCommands(
	eventStore domain.EventStore[spell.ID, spell.Event, spell.State],
	identifier domain.Identifier[spell.ID],
	updateName spell.UpdateNameFunc,
	now func() time.Time,
) *SpellCommands {
	return &SpellCommands{eventStore: eventStore, identifier: identifier, updateName: updateName, now: now}
}

// uif にドメインの型（spell.Spell・card.Name）を見せないため、表示に要る値だけを返す
type RegisteredSpell struct {
	ID           uuid.UUID
	Name         string
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// 操作した人は context のオペレーター（認証ミドルウェアが載せる）。入力の誤りは input.Errors、
// 使われている名前なら ErrCardNameAlreadyInUse
func (c *SpellCommands) Register(ctx context.Context, name string) (RegisteredSpell, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return RegisteredSpell{}, ErrNotOperator
	}
	cardName, invalid := newCardName("name", name)
	if invalid != nil {
		return RegisteredSpell{}, input.Errors{invalid}
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (spell.Spell, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return spell.Spell{}, err
		}
		if err := c.updateName(ctx, id, cardName); err != nil {
			return spell.Spell{}, err
		}
		cmd := &spell.RegisterCommand{ID: id, PublicID: publicID, Name: cardName, RegisteredBy: registeredBy}
		events, err := spell.Decide(spell.InitialState(), cmd, c.now())
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
		ID: registered.PublicID, Name: registered.Name.String(),
		RegisteredAt: registered.RegisteredAt, UpdatedAt: registered.RegisteredAt,
	}, nil
}

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
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

// uif にドメインの型（item.Item・card.Name）を見せないため、表示に要る値だけを返す
type RegisteredItem struct {
	ID           uuid.UUID
	Name         string
	RegisteredAt time.Time
	UpdatedAt    time.Time
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
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (item.Item, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return item.Item{}, err
		}
		if err := c.updateName(ctx, id, cardName); err != nil {
			return item.Item{}, err
		}
		cmd := &item.RegisterCommand{ID: id, PublicID: publicID, Name: cardName, RegisteredBy: registeredBy}
		events, err := item.Decide(item.InitialState(), cmd, c.now())
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
		ID: registered.PublicID, Name: registered.Name.String(),
		RegisteredAt: registered.RegisteredAt, UpdatedAt: registered.RegisteredAt,
	}, nil
}

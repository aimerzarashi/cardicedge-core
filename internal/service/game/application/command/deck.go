package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type DeckHandler struct {
	eventStore    domain.EventStore[deck.ID, deck.Event, deck.State]
	identifier    domain.Identifier[deck.ID]
	updateStarter deck.UpdateStarterFunc
	now           func() time.Time
}

func NewDeckHandler(
	eventStore domain.EventStore[deck.ID, deck.Event, deck.State],
	identifier domain.Identifier[deck.ID],
	updateStarter deck.UpdateStarterFunc,
	now func() time.Time,
) *DeckHandler {
	return &DeckHandler{eventStore: eventStore, identifier: identifier, updateStarter: updateStarter, now: now}
}

// 配布の number 番目のデッキを作る。もうあれば（配布のやり直し）、何もしない
func (c *DeckHandler) Create(ctx context.Context, owner player.ID, grant startergrant.ID, number startergrant.DeckNumber, source startergrant.Deck) error {
	name, err := deck.NewName(source.Name.String())
	if err != nil {
		return err
	}
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, _, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.updateStarter(ctx, id, grant, number); err != nil {
			return struct{}{}, err
		}
		events, err := deck.Decide(deck.InitialState(), &deck.CreateCommand{
			ID: id, Owner: owner, Name: name, Cards: source.Cards, Grant: grant, Number: number,
		}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, deck.ErrAlreadyCreated) {
		return nil
	}
	return err
}

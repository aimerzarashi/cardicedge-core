package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type CollectionHandler struct {
	eventStore  domain.EventStore[collection.ID, collection.Event, collection.State]
	identifier  domain.Identifier[collection.ID]
	updateOwner collection.UpdateOwnerFunc
	now         func() time.Time
}

func NewCollectionHandler(
	eventStore domain.EventStore[collection.ID, collection.Event, collection.State],
	identifier domain.Identifier[collection.ID],
	updateOwner collection.UpdateOwnerFunc,
	now func() time.Time,
) *CollectionHandler {
	return &CollectionHandler{eventStore: eventStore, identifier: identifier, updateOwner: updateOwner, now: now}
}

// 所持カードを作る。そのプレイヤーの所持カードがもうあれば（配布のやり直し）、何もしない
func (c *CollectionHandler) Create(ctx context.Context, owner player.ID, grant startergrant.ID, cards []startergrant.CardCount) error {
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, _, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.updateOwner(ctx, id, owner); err != nil {
			return struct{}{}, err
		}
		events, err := collection.Decide(collection.InitialState(), &collection.CreateCommand{ID: id, Owner: owner, Cards: cards, Grant: grant}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, collection.ErrAlreadyCreated) {
		return nil
	}
	return err
}

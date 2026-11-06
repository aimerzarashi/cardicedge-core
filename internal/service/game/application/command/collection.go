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
	eventStore   domain.EventStore[collection.ID, collection.Event, collection.State]
	identifier   domain.Identifier[collection.ID]
	updatePlayer collection.UpdatePlayerFunc
	findByPlayer FindCollectionByPlayerFunc
	now          func() time.Time
}

type FindCollectionByPlayerFunc func(ctx context.Context, playerID player.ID) (collection.ID, bool, error)

// プレイヤーの所持品がまだ無い（スターターの配布が済む前）。配布が済めば直る失敗なので、ポリシーのやり直しに任せる
var ErrCollectionNotCreated = errors.New("command: collection not created")

func NewCollectionHandler(
	eventStore domain.EventStore[collection.ID, collection.Event, collection.State],
	identifier domain.Identifier[collection.ID],
	updatePlayer collection.UpdatePlayerFunc,
	findByPlayer FindCollectionByPlayerFunc,
	now func() time.Time,
) *CollectionHandler {
	return &CollectionHandler{eventStore: eventStore, identifier: identifier, updatePlayer: updatePlayer, findByPlayer: findByPlayer, now: now}
}

// 所持品（いまはカード）を作る。そのプレイヤーの所持品がもうあれば（配布のやり直し）、何もしない
func (c *CollectionHandler) Create(ctx context.Context, playerID player.ID, starterGrantID startergrant.ID, cards []collection.Card) error {
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, _, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.updatePlayer(ctx, id, playerID); err != nil {
			return struct{}{}, err
		}
		events, err := collection.Decide(collection.InitialState(), &collection.CreateCommand{ID: id, Player: playerID, Cards: cards, StarterGrant: starterGrantID}, c.now())
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

// プレイヤーの所持品にカードを足す。同じ出どころ（同じ対戦の戦利品）の分はもう足してあれば何もしない（ポリシーは二度動くことがある）。
// 足すカードが無ければ何もしない。所持品がまだ無ければ ErrCollectionNotCreated
func (c *CollectionHandler) Acquire(ctx context.Context, playerID player.ID, cards []collection.Card, source collection.Source) error {
	if len(cards) == 0 {
		return nil
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, found, err := c.findByPlayer(ctx, playerID)
		if err != nil {
			return struct{}{}, err
		}
		if !found {
			return struct{}{}, ErrCollectionNotCreated
		}
		state, err := c.eventStore.Load(ctx, id)
		if err != nil {
			return struct{}{}, err
		}
		events, err := collection.Decide(state, &collection.AcquireCommand{Cards: cards, Source: source}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, collection.ErrAlreadyAcquired) {
		return nil
	}
	return err
}

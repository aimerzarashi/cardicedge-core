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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

type DeckHandler struct {
	eventStore    domain.EventStore[deck.ID, deck.Event, deck.State]
	identifier    domain.Identifier[deck.ID]
	updateStarter deck.UpdateStarterFunc
	updateName    deck.UpdateNameFunc
	reserveQuota  deck.ReserveQuotaFunc
	getRules      rules.GetRulesFunc
	getQuotaLimit rules.GetQuotaLimitFunc
	now           func() time.Time
}

func NewDeckHandler(
	eventStore domain.EventStore[deck.ID, deck.Event, deck.State],
	identifier domain.Identifier[deck.ID],
	updateStarter deck.UpdateStarterFunc,
	updateName deck.UpdateNameFunc,
	reserveQuota deck.ReserveQuotaFunc,
	getQuotaLimit rules.GetQuotaLimitFunc,
	getRules rules.GetRulesFunc,
	now func() time.Time,
) *DeckHandler {
	return &DeckHandler{
		eventStore: eventStore, identifier: identifier, updateStarter: updateStarter, updateName: updateName,
		reserveQuota: reserveQuota, getQuotaLimit: getQuotaLimit, getRules: getRules, now: now,
	}
}

// 配布の number 番目のデッキを作る。もうあれば（配布のやり直し）、何もしない。
// 制約は 1 つのトランザクションの中で、配布 × 番号（重なれば何もしない）→ 名前 → 上限の順に確保する。
// 配布 × 番号が重なれば全体が戻るので、上限の数は増えない。名前の重なり・上限は、スターターの配布では起きない
// （コードの誤り）ため、エラーのまま返す
func (c *DeckHandler) Create(ctx context.Context, playerID player.ID, starterGrantID startergrant.ID, number startergrant.DeckNumber, source startergrant.Deck) error {
	name, err := deck.NewName(source.Name.String())
	if err != nil {
		return err
	}
	// 決まりの枚数は、作るときのルールで確かめる（配布を始めた時点のルールの写しではなく、今のルール。デッキの決まりはデッキを作る側が守る）
	gameRules, found, err := c.getRules(ctx)
	if err != nil {
		return err
	}
	if !found {
		return rules.ErrNotPublished
	}
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, _, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.updateStarter(ctx, id, starterGrantID, number); err != nil {
			return struct{}{}, err
		}
		if err := c.updateName(ctx, id, playerID, name); err != nil {
			return struct{}{}, err
		}
		limit, err := c.getQuotaLimit(ctx, deck.PlayerQuotaType)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.reserveQuota(ctx, deck.PlayerQuota(playerID), limit); err != nil {
			return struct{}{}, err
		}
		cards := make([]deck.Card, len(source.Cards))
		for i, c := range source.Cards {
			cards[i] = deck.Card{Card: c.Card, Quantity: c.Quantity}
		}
		events, err := deck.Decide(deck.InitialState(), &deck.CreateCommand{
			ID: id, Player: playerID, Name: name, Cards: cards, Total: gameRules.Deck.TotalCardCount, StarterGrant: starterGrantID, Number: number,
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

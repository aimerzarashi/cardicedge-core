package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// 版の衝突でやり直す回数の上限。衝突するのは、所持カードとデッキの記録が同じ配布に同時に来たときだけ（最大で、配るデッキの数 + 1 つ。既定の公開中の上限 2 なら 3 つ）
const maxRecordAttempts = 5

type StarterGrantHandler struct {
	eventStore       domain.EventStore[startergrant.ID, startergrant.Event, startergrant.State]
	identifier       domain.Identifier[startergrant.ID]
	updatePlayer     startergrant.UpdatePlayerFunc
	listStarterDecks master.ListPublishedStarterDecksFunc
	collections      *CollectionHandler
	decks            *DeckHandler
	now              func() time.Time
}

func NewStarterGrantHandler(
	eventStore domain.EventStore[startergrant.ID, startergrant.Event, startergrant.State],
	identifier domain.Identifier[startergrant.ID],
	updatePlayer startergrant.UpdatePlayerFunc,
	listStarterDecks master.ListPublishedStarterDecksFunc,
	collections *CollectionHandler,
	decks *DeckHandler,
	now func() time.Time,
) *StarterGrantHandler {
	return &StarterGrantHandler{
		eventStore: eventStore, identifier: identifier, updatePlayer: updatePlayer, listStarterDecks: listStarterDecks,
		collections: collections, decks: decks, now: now,
	}
}

// 配布の流れ（プロセスマネージャー）の、イベントを受け取る入口。eventdriven のポリシーとして game が登録する。
// どのイベントで次に何をするかは配布の流れそのものなので、流れを持つこのハンドラーの中で振り分ける
// （ポリシーを presentation 層に置くとドメインのイベントの型を import することになり、application 層に別のパッケージで置くと、
// application 層のユースケースが別のパッケージのユースケースを呼ぶ形になるため）。購読する集約のほかのイベントは何もしない
func (c *StarterGrantHandler) Handle(ctx context.Context, event any) error {
	switch e := event.(type) {
	case *player.RegisteredEvent:
		return c.Start(ctx, e.GetMeta().EntityID.Int64())
	case *startergrant.StartedEvent:
		return c.Distribute(ctx, e.GetMeta().EntityID.Int64())
	case *collection.CreatedEvent:
		return c.RecordCollection(ctx, e.StarterGrant.Int64())
	case *deck.CreatedEvent:
		return c.RecordDeck(ctx, e.StarterGrant.Int64(), e.Number.Int())
	default:
		return nil
	}
}

// プレイヤーの配布を始める。公開中のスターターデッキが 1 つも無ければ ErrMasterNotPublished（公開中をあるだけ配る）。そのプレイヤーの配布がもう始まっていれば何もしない
func (c *StarterGrantHandler) Start(ctx context.Context, rawPlayerID int64) error {
	playerID, err := player.NewID(rawPlayerID)
	if err != nil {
		return err
	}
	published, err := c.listStarterDecks(ctx)
	if err != nil {
		return err
	}
	if len(published) == 0 {
		return ErrMasterNotPublished
	}
	decks, err := startDecks(published)
	if err != nil {
		return err
	}
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, _, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.updatePlayer(ctx, id, playerID); err != nil {
			return struct{}{}, err
		}
		events, err := startergrant.Decide(startergrant.InitialState(), &startergrant.StartCommand{ID: id, Player: playerID, Decks: decks}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, startergrant.ErrAlreadyStarted) {
		return nil
	}
	return err
}

// 所持カードと公開中のスターターデッキの数だけのデッキを、それぞれ別のトランザクションで作る（どのトランザクションも 1 つの集約だけを変える）。
// どれも、もうあれば何もしないので、途中で止まっても、もう一度呼べば続きから進む
func (c *StarterGrantHandler) Distribute(ctx context.Context, grantID int64) error {
	state, err := c.load(ctx, grantID)
	if err != nil {
		return err
	}
	switch s := state.(type) {
	case startergrant.CompletedStarterGrant:
		return nil
	case startergrant.InProgressStarterGrant:
		if err := c.collections.Create(ctx, s.Player, s.ID, s.Cards); err != nil {
			return err
		}
		for i, source := range s.Decks {
			number, err := startergrant.NewDeckNumber(i + 1)
			if err != nil {
				return err
			}
			if err := c.decks.Create(ctx, s.Player, s.ID, number, source); err != nil {
				return err
			}
		}
		return nil
	default:
		return startergrant.ErrInvalidState
	}
}

// 所持カードができたことを記録する
func (c *StarterGrantHandler) RecordCollection(ctx context.Context, grantID int64) error {
	return c.record(ctx, grantID, &startergrant.RecordCollectionCommand{})
}

// number 番目のデッキができたことを記録する
func (c *StarterGrantHandler) RecordDeck(ctx context.Context, grantID int64, number int) error {
	deckNumber, err := startergrant.NewDeckNumber(number)
	if err != nil {
		return err
	}
	return c.record(ctx, grantID, &startergrant.RecordDeckCommand{Number: deckNumber})
}

// 版がぶつかったら読み直してやり直す。所持カードとデッキの記録が同じ配布に同時に来ることがあり、
// ぶつかるたびにポリシーの失敗にすると、やり直しの間隔を待つ分だけ配り終わるのが遅れるため
func (c *StarterGrantHandler) record(ctx context.Context, grantID int64, cmd startergrant.Command) error {
	for range maxRecordAttempts {
		state, err := c.load(ctx, grantID)
		if err != nil {
			return err
		}
		events, err := startergrant.Decide(state, cmd, c.now())
		if err != nil || len(events) == 0 {
			return err
		}
		err = c.eventStore.Save(ctx, events)
		if !errors.Is(err, domain.ErrVersionConflict) {
			return err
		}
	}
	return domain.ErrVersionConflict
}

func (c *StarterGrantHandler) load(ctx context.Context, grantID int64) (startergrant.State, error) {
	id, err := startergrant.NewID(grantID)
	if err != nil {
		return nil, err
	}
	return c.eventStore.Load(ctx, id)
}

func startDecks(published []master.StarterDeck) ([]startergrant.Deck, error) {
	decks := make([]startergrant.Deck, 0, len(published))
	for _, source := range published {
		name, err := startergrant.NewDeckName(source.Name)
		if err != nil {
			return nil, fmt.Errorf("master starter deck name %q: %w", source.Name, err)
		}
		cards := make([]startergrant.CardCount, 0, len(source.Cards))
		for _, c := range source.Cards {
			count, err := startergrant.NewCount(c.Count)
			if err != nil {
				return nil, fmt.Errorf("master starter deck %q card %s: %w", source.Name, c.CardID, err)
			}
			cards = append(cards, startergrant.CardCount{CardID: c.CardID, Count: count})
		}
		decks = append(decks, startergrant.Deck{Name: name, Cards: cards})
	}
	return decks, nil
}

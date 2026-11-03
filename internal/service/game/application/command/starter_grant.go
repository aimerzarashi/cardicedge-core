package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// 版の衝突でやり直す回数の上限。衝突するのは、所持カード・解放・デッキの記録が同じ配布に同時に来たときだけ（最大で、配るデッキの数 + 2 つ。デッキが 2 つなら 4 つ）
const maxRecordAttempts = 5

type StarterGrantHandler struct {
	eventStore    domain.EventStore[startergrant.ID, startergrant.Event, startergrant.State]
	identifier    domain.Identifier[startergrant.ID]
	updatePlayer  startergrant.UpdatePlayerFunc
	getStarterSet master.GetPublishedStarterSetFunc
	collections   *CollectionHandler
	unlocks       *UnlockHandler
	findGrant     FindStarterGrantByPlayerFunc
	decks         *DeckHandler
	now           func() time.Time
}

func NewStarterGrantHandler(
	eventStore domain.EventStore[startergrant.ID, startergrant.Event, startergrant.State],
	identifier domain.Identifier[startergrant.ID],
	updatePlayer startergrant.UpdatePlayerFunc,
	getStarterSet master.GetPublishedStarterSetFunc,
	collections *CollectionHandler,
	unlocks *UnlockHandler,
	findGrant FindStarterGrantByPlayerFunc,
	decks *DeckHandler,
	now func() time.Time,
) *StarterGrantHandler {
	return &StarterGrantHandler{
		eventStore: eventStore, identifier: identifier, updatePlayer: updatePlayer, getStarterSet: getStarterSet,
		collections: collections, unlocks: unlocks, findGrant: findGrant, decks: decks, now: now,
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
	case *unlock.CreatedEvent:
		return c.UnlockSnapshotBoards(ctx, e.GetMeta().EntityID, e.Player)
	case *unlock.BoardsUnlockedEvent:
		// 実績などで後から解放したものも来る。配布が済んでいれば、record が何もしない
		playerID, err := c.unlocks.Player(ctx, e.GetMeta().EntityID)
		if err != nil {
			return err
		}
		return c.recordUnlockOf(ctx, playerID)
	case *deck.CreatedEvent:
		return c.RecordDeck(ctx, e.StarterGrant.Int64(), e.Number.Int())
	default:
		return nil
	}
}

// プレイヤーの配布を始める。公開中のスターターセットが無い（配るデッキが公開中のものが 1 つも無い場合を含む）なら ErrMasterNotPublished。
// セットが指す配布デッキを、セットの順に配り、セットが指すボードを解放する。そのプレイヤーの配布がもう始まっていれば何もしない
func (c *StarterGrantHandler) Start(ctx context.Context, rawPlayerID int64) error {
	playerID, err := player.NewID(rawPlayerID)
	if err != nil {
		return err
	}
	set, found, err := c.getStarterSet(ctx)
	if err != nil {
		return err
	}
	if !found || len(set.GrantDecks) == 0 {
		return ErrMasterNotPublished
	}
	decks, err := startDecks(set.GrantDecks)
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
		events, err := startergrant.Decide(startergrant.InitialState(), &startergrant.StartCommand{ID: id, Player: playerID, Decks: decks, UnlockBoards: set.UnlockBoards}, c.now())
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

// 所持カード・解放と、セットが指す配布デッキの数だけのデッキを、それぞれ別のトランザクションで作る（どのトランザクションも 1 つの集約だけを変える）。
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
		return c.unlocks.Create(ctx, s.Player)
	default:
		return startergrant.ErrInvalidState
	}
}

// 所持カードができたことを記録する
func (c *StarterGrantHandler) RecordCollection(ctx context.Context, grantID int64) error {
	return c.record(ctx, grantID, &startergrant.RecordCollectionCommand{})
}

// 解放の集約ができたら、プレイヤーの配布を引き、写したボードを解放する（配布の流れの段）。ボードが無ければ、解放の段を済んだものとして
// 記録する。ボードがすべて解放済みでイベントが出ないときも、BoardsUnlockedEvent が来ないので、ここで記録する（記録は何度呼んでもよい）。
// 配布が無い・済んでいる・解放の段が済んでいるときは何もしない
func (c *StarterGrantHandler) UnlockSnapshotBoards(ctx context.Context, unlockID unlock.ID, playerID player.ID) error {
	grantID, found, err := c.findGrant(ctx, playerID)
	if err != nil || !found {
		return err
	}
	state, err := c.load(ctx, grantID.Int64())
	if err != nil {
		return err
	}
	progress, ok := state.(startergrant.InProgressStarterGrant)
	if !ok || progress.UnlockRecorded {
		return nil
	}
	if len(progress.UnlockBoards) > 0 {
		unlocked, err := c.unlocks.UnlockBoards(ctx, unlockID, progress.UnlockBoards)
		if err != nil || unlocked {
			return err
		}
	}
	return c.RecordUnlock(ctx, grantID.Int64())
}

// プレイヤーの配布の解放の段を記録する。配布が無ければ何もしない
func (c *StarterGrantHandler) recordUnlockOf(ctx context.Context, playerID player.ID) error {
	grantID, found, err := c.findGrant(ctx, playerID)
	if err != nil || !found {
		return err
	}
	return c.RecordUnlock(ctx, grantID.Int64())
}

// 解放ができたことを記録する
func (c *StarterGrantHandler) RecordUnlock(ctx context.Context, grantID int64) error {
	return c.record(ctx, grantID, &startergrant.RecordUnlockCommand{})
}

// number 番目のデッキができたことを記録する
func (c *StarterGrantHandler) RecordDeck(ctx context.Context, grantID int64, number int) error {
	deckNumber, err := startergrant.NewDeckNumber(number)
	if err != nil {
		return err
	}
	return c.record(ctx, grantID, &startergrant.RecordDeckCommand{Number: deckNumber})
}

// 版がぶつかったら読み直してやり直す。所持カード・解放・デッキの記録が同じ配布に同時に来ることがあり、
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

// プレイヤーの配布を引く（プレイヤーに 1 つ）。無ければ found が false
type FindStarterGrantByPlayerFunc func(ctx context.Context, playerID player.ID) (startergrant.ID, bool, error)

func startDecks(published []master.GrantDeck) ([]startergrant.Deck, error) {
	decks := make([]startergrant.Deck, 0, len(published))
	for _, source := range published {
		name, err := startergrant.NewDeckName(source.Name)
		if err != nil {
			return nil, fmt.Errorf("master grant deck name %q: %w", source.Name, err)
		}
		cards := make([]card.Count, 0, len(source.Cards))
		for _, c := range source.Cards {
			count, err := card.NewQuantity(c.Count)
			if err != nil {
				return nil, fmt.Errorf("master grant deck %q card %s: %w", source.Name, c.CardID, err)
			}
			cards = append(cards, card.Count{CardID: c.CardID, Count: count})
		}
		decks = append(decks, startergrant.Deck{Name: name, Cards: cards})
	}
	return decks, nil
}

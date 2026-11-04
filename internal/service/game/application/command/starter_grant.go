package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

// 版の衝突でやり直す回数の上限。衝突するのは、所持カード・解放・デッキの記録が同じ配布に同時に来たときだけ（最大で、配るデッキの数 + 2 つ。デッキが 2 つなら 4 つ）
const maxRecordAttempts = 5

type StarterGrantHandler struct {
	eventStore    domain.EventStore[startergrant.ID, startergrant.Event, startergrant.State]
	identifier    domain.Identifier[startergrant.ID]
	updatePlayer  startergrant.UpdatePlayerFunc
	getStarterSet master.GetPublishedStarterSetFunc
	getRules      rules.GetRulesFunc
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
	getRules rules.GetRulesFunc,
	collections *CollectionHandler,
	unlocks *UnlockHandler,
	findGrant FindStarterGrantByPlayerFunc,
	decks *DeckHandler,
	now func() time.Time,
) *StarterGrantHandler {
	return &StarterGrantHandler{
		eventStore: eventStore, identifier: identifier, updatePlayer: updatePlayer, getStarterSet: getStarterSet, getRules: getRules,
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
		return c.UnlockSnapshot(ctx, e.GetMeta().EntityID, e.Player)
	// 実績などで後から解放したものも来る。配布が済んでいる・解放の段が済んでいれば、何もしない
	case *unlock.BoardsUnlockedEvent:
		return c.OnUnlocked(ctx, e.GetMeta().EntityID)
	case *unlock.NPCAvatarsUnlockedEvent:
		return c.OnUnlocked(ctx, e.GetMeta().EntityID)
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
	gameRules, found, err := c.getRules(ctx)
	if err != nil {
		return err
	}
	if !found {
		return rules.ErrNotPublished
	}
	decks, err := startDecks(set.GrantDecks, gameRules.Deck.MaxCardDuplicates)
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
		events, err := startergrant.Decide(startergrant.InitialState(), &startergrant.StartCommand{ID: id, Player: playerID, Decks: decks, UnlockBoards: set.UnlockBoards, UnlockNPCAvatars: set.UnlockNPCAvatars}, c.now())
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
		owned := make([]collection.Card, len(s.Cards))
		for i, entry := range s.Cards {
			owned[i] = collection.Card{Card: entry.Card, Quantity: entry.Quantity}
		}
		if err := c.collections.Create(ctx, s.Player, s.ID, owned); err != nil {
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

// 解放の集約ができたら、プレイヤーの配布を引き、写したボードと NPC のアバターを 1 つのコマンドで解放する（配布の流れの段）。
// 解放の段は、写したものがすべて解放されたときに記録する（UnlockedSnapshot）ので、種類が何であれ、解放するものが無い・
// すべて解放済みでイベントが出ないときも、ここで記録する（記録は何度呼んでもよい）。
// 配布が無い・済んでいる・解放の段が済んでいるときは何もしない
func (c *StarterGrantHandler) UnlockSnapshot(ctx context.Context, unlockID unlock.ID, playerID player.ID) error {
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
	if len(progress.UnlockBoards) > 0 || len(progress.UnlockNPCAvatars) > 0 {
		if _, err := c.unlocks.Unlock(ctx, unlockID, progress.UnlockBoards, progress.UnlockNPCAvatars); err != nil {
			return err
		}
	}
	return c.recordUnlockIfUnlocked(ctx, grantID, unlockID)
}

// 解放のイベント（ボード・NPC のどちらも）を受けたら、unlock の持ち主から配布を引き、写したものがそろっていれば解放の段を記録する。
// 配布が済んでいる・段が記録済みなら何もしない（実績などで後から解放したものは無視する）
func (c *StarterGrantHandler) OnUnlocked(ctx context.Context, unlockID unlock.ID) error {
	current, err := c.unlocks.Get(ctx, unlockID)
	if err != nil {
		return err
	}
	grantID, found, err := c.findGrant(ctx, current.Player)
	if err != nil || !found {
		return err
	}
	return c.recordUnlockIfUnlocked(ctx, grantID, unlockID)
}

// 配布に写したボードと NPC のアバターが、解放の集約にすべてあれば、解放の段を記録する
func (c *StarterGrantHandler) recordUnlockIfUnlocked(ctx context.Context, grantID startergrant.ID, unlockID unlock.ID) error {
	state, err := c.load(ctx, grantID.Int64())
	if err != nil {
		return err
	}
	progress, ok := state.(startergrant.InProgressStarterGrant)
	if !ok || progress.UnlockRecorded {
		return nil
	}
	current, err := c.unlocks.Get(ctx, unlockID)
	if err != nil {
		return err
	}
	if !containsAll(current.Boards, progress.UnlockBoards) || !containsAll(current.NPCAvatars, progress.UnlockNPCAvatars) {
		return nil
	}
	return c.RecordUnlock(ctx, grantID.Int64())
}

func containsAll[T comparable](have, want []T) bool {
	for _, id := range want {
		if !slices.Contains(have, id) {
			return false
		}
	}
	return true
}

// プレイヤーの配布の解放の段を記録する。配布が無ければ何もしない
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

func startDecks(published []master.GrantDeck, maxCardDuplicates card.MaxDeckQuantity) ([]startergrant.Deck, error) {
	decks := make([]startergrant.Deck, 0, len(published))
	for _, source := range published {
		name, err := startergrant.NewDeckName(source.Name)
		if err != nil {
			return nil, fmt.Errorf("master grant deck name %q: %w", source.Name, err)
		}
		cards := make([]startergrant.DeckCard, 0, len(source.Cards))
		for _, c := range source.Cards {
			quantity, err := card.NewDeckQuantity(c.Count, maxCardDuplicates)
			if err != nil {
				return nil, fmt.Errorf("master grant deck %q card %s/%d: %w", source.Name, c.Card.EntityType, c.Card.ID, err)
			}
			cards = append(cards, startergrant.DeckCard{Card: c.Card, Quantity: quantity})
		}
		decks = append(decks, startergrant.Deck{Name: name, Cards: cards})
	}
	return decks, nil
}

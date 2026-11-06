// game 用 GraphQL の部品（リゾルバー・入口）
package game

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"io"
	"math/rand/v2"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql"
)

// JWT の検証は外の世界（発行元の JWKS）に触れるため、作ったものを受け取る（テストでは idptest の鍵で作る）
type Config struct {
	Verifier *idp.Verifier
	Issuer   string
	// 対戦の進み（Subscription）を、変わらなくても送り直す間隔。通知の取りこぼしを埋める。環境ごとに変える値ではなく、
	// テストが短くできるように組み立てで渡す
	MatchProgressInterval time.Duration
}

type Game struct {
	config          Config
	entityRefs      domain.EntityRefResolver
	players         *command.PlayerHandler
	grants          *command.StarterGrantHandler
	matchStart      *command.MatchStartHandler
	matchNPCTurn    *command.MatchNPCTurnHandler
	matchReward     *command.MatchRewardHandler
	roomMatchFinish *command.RoomMatchFinishHandler
	matches         *command.MatchHandler
	matchQuery      *query.MatchHandler
	matchEvents     *query.MatchEventsHandler
	matchProgress   *query.MatchProgressBroker
	playerQuery     *query.PlayerHandler
	avatars         *command.AvatarHandler
	avatarQuery     *query.AvatarHandler
	deckQuery       *query.DeckHandler
	unlockQuery     *query.UnlockHandler
	boardQuery      *query.BoardHandler
	gameRuleQuery   *query.GameRulesHandler
	npcAvatarQuery  *query.NPCAvatarHandler
	rooms           *command.RoomHandler
	roomQuery       *query.RoomHandler
}

func New(registry *domaindef.Registry, config Config) *Game {
	// DB（timestamptz）はマイクロ秒までしか持たないため、そろえておく（studio と同じ）
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	boardReader := repository.NewBoardReader()
	npcAvatarReader := repository.NewNPCAvatarReader()
	unlockReader := repository.NewUnlockReader()
	entityRefs := identifier.NewEntityRefResolver(registry)
	gameRulesReader := repository.NewGameRulesReader()
	avatarQuery := query.NewAvatarHandler(repository.NewAvatarReader().ListByPlayer)
	deckReader := repository.NewDeckReader()
	deckQuery := query.NewDeckHandler(deckReader.ListByPlayer, deckReader.Get)
	unlockQuery := query.NewUnlockHandler(unlockReader.ListBoardsByPlayer)
	npcAvatarQuery := query.NewNPCAvatarHandler(npcAvatarReader.ListUnlockedByPlayer, npcAvatarReader.ListPublishedDecks)
	roomReader := repository.NewRoomReader()
	matchSources := repository.NewMatchSourceReader(entityRefs)
	matchStore := eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve)
	matchAccess := query.NewMatchAccess(matchStore, identifier.New(registry, match.NewID), identifier.New(registry, player.NewID))
	matchEventLoader := eventstore.NewEventLoader(registry, match.NewID, match.NewEventType)
	matchNames := repository.NewMatchNameReader()
	collections := command.NewCollectionHandler(
		eventstore.New(registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve),
		identifier.New(registry, collection.NewID),
		constraint.UpdateCollectionPlayer,
		constraint.FindCollectionByPlayer,
		now,
	)
	return &Game{
		config:     config,
		entityRefs: entityRefs,
		players: command.NewPlayerHandler(
			eventstore.New(registry, player.NewID, player.NewEventType, player.InitialState, player.Evolve),
			identifier.New(registry, player.NewID),
			constraint.UpdatePlayerSubject,
			constraint.FindPlayerBySubject,
			now,
		),
		grants: command.NewStarterGrantHandler(
			eventstore.New(registry, startergrant.NewID, startergrant.NewEventType, startergrant.InitialState, startergrant.Evolve),
			identifier.New(registry, startergrant.NewID),
			constraint.UpdateStarterGrantPlayer,
			repository.NewStarterSetReader(repository.NewGrantDeckReader(entityRefs), identifier.New(registry, board.NewID), identifier.New(registry, npcavatar.NewID)).GetPublished,
			repository.NewGameRulesReader().Get,
			collections,
			command.NewUnlockHandler(
				eventstore.New(registry, unlock.NewID, unlock.NewEventType, unlock.InitialState, unlock.Evolve),
				identifier.New(registry, unlock.NewID),
				constraint.UpdateUnlockPlayer,
				now,
			),
			constraint.FindStarterGrantByPlayer,
			command.NewDeckHandler(
				eventstore.New(registry, deck.NewID, deck.NewEventType, deck.InitialState, deck.Evolve),
				identifier.New(registry, deck.NewID),
				constraint.UpdateDeckStarter,
				constraint.UpdateDeckName,
				constraint.ReserveDeckQuota,
				rules.FromPublished(repository.NewGameRulesReader().Get),
				repository.NewGameRulesReader().Get,
				now,
			),
			now,
		),
		matchStart: command.NewMatchStartHandler(
			eventstore.New(registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
			eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve),
			gameRulesReader.Get,
			matchSources.Board,
			matchSources.PlayerDeckCards,
			matchSources.NPCDeckCards,
			newRandom,
			now,
		),
		matchNPCTurn: command.NewMatchNPCTurnHandler(
			eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve),
			newRandom,
			now,
		),
		matchReward: command.NewMatchRewardHandler(
			eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve),
			collections,
		),
		roomMatchFinish: command.NewRoomMatchFinishHandler(
			eventstore.New(registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
			identifier.New(registry, room.NewID),
			entityRefs,
			roomReader.FindByMatch,
			now,
		),
		matches: command.NewMatchHandler(
			eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve),
			identifier.New(registry, match.NewID),
			matchSources.PublishedCards,
			newRandom,
			now,
		),
		matchQuery: query.NewMatchHandler(
			eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve),
			identifier.New(registry, match.NewID),
			identifier.New(registry, player.NewID),
			entityRefs,
			repository.NewMatchNameReader().Load,
			roomReader.FindByMatch,
		),
		matchEvents: query.NewMatchEventsHandler(matchAccess.Authorize, func(ctx context.Context, id match.ID) ([]match.Event, error) {
			loaded, err := matchEventLoader.LoadEvents(ctx, id.Int64())
			if err != nil {
				return nil, err
			}
			events := make([]match.Event, len(loaded))
			for i, event := range loaded {
				events[i] = event.Event.(match.Event)
			}
			return events, nil
		}, entityRefs, matchNames.Load),
		matchProgress: query.NewMatchProgressBroker(matchAccess.Authorize, repository.NewMatchHeadReader().Load, config.MatchProgressInterval),
		playerQuery:   query.NewPlayerHandler(repository.NewPlayerReader().GetStarterGrantStatus),
		avatars: command.NewAvatarHandler(
			eventstore.New(registry, avatar.NewID, avatar.NewEventType, avatar.InitialState, avatar.Evolve),
			identifier.New(registry, avatar.NewID),
			constraint.UpdateAvatarName,
			constraint.ReserveAvatarQuota,
			rules.FromPublished(repository.NewGameRulesReader().Get),
			now,
		),
		avatarQuery:    avatarQuery,
		deckQuery:      deckQuery,
		unlockQuery:    unlockQuery,
		boardQuery:     query.NewBoardHandler(boardReader.ListPublished, boardReader.GetPublished),
		gameRuleQuery:  query.NewGameRulesHandler(gameRulesReader.Get),
		npcAvatarQuery: npcAvatarQuery,
		rooms: command.NewRoomHandler(
			eventstore.New(registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
			identifier.New(registry, room.NewID),
			identifier.New(registry, match.NewID),
			command.RoomReferences{
				Avatars:    identifier.New(registry, avatar.NewID),
				Decks:      identifier.New(registry, deck.NewID),
				NPCAvatars: identifier.New(registry, npcavatar.NewID),
				NPCDecks:   identifier.New(registry, npcdeck.NewID),
				Boards:     identifier.New(registry, board.NewID),
			},
			entityRefs,
			// 席・対戦ルールに選べるものは、画面の選択肢を出す query（Player.avatars・decks・npcAvatars・boards）と同じハンドラーを通して調べる
			query.NewRoomSelectable(avatarQuery, deckQuery, npcAvatarQuery, unlockQuery),
			gameRulesReader.Get,
			constraint.CountQuota,
			now,
		),
		// できる操作は Decide と同じ関数で答えるため、read model ではなくイベントストアから読む集約の状態を使う
		roomQuery: query.NewRoomHandler(roomReader.ListByPlayer, roomReader.Get, query.NewRoomCapabilitiesLoader(
			eventstore.New(registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
			identifier.New(registry, room.NewID),
			identifier.New(registry, player.NewID),
		).Get, query.NewRoomRequirementsLoader(constraint.CountQuota, identifier.New(registry, player.NewID)).Get),
	}
}

// 対戦ごとの乱数の元。予測されない種を OS の乱数から取る（順番と山札の並びを、先に読めないようにするため）。
// 種の取得に失敗するのは OS の乱数が使えないときだけで、続けても安全な代わりが無いので止める
func newRandom() match.Random {
	var seed [32]byte
	if _, err := crand.Read(seed[:]); err != nil {
		panic(err)
	}
	return rand.New(rand.NewChaCha8(seed))
}

// 名前と購読は internal/wiring の購読者の一覧と照合される（studio の RegisterProjections と同じ形）。最初の失敗で止める
func (g *Game) RegisterPolicies(hub *eventdriven.Hub) error {
	if err := eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{
		Name:          "match_start",
		Subscriptions: []domain.EntityType{room.Types().EntityType},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			return g.matchStart.Handle(ctx, event.Event)
		},
	}); err != nil {
		return err
	}
	if err := eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{
		Name:          "match_npc_turn",
		Subscriptions: []domain.EntityType{match.Types().EntityType},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			return g.matchNPCTurn.Handle(ctx, event.Event)
		},
	}); err != nil {
		return err
	}
	if err := eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{
		Name:          "match_reward",
		Subscriptions: []domain.EntityType{match.Types().EntityType},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			return g.matchReward.Handle(ctx, event.Event)
		},
	}); err != nil {
		return err
	}
	if err := eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{
		Name:          "room_match_finish",
		Subscriptions: []domain.EntityType{match.Types().EntityType},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			return g.roomMatchFinish.Handle(ctx, event.Event)
		},
	}); err != nil {
		return err
	}
	return eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{
		Name: "starter_grant",
		Subscriptions: []domain.EntityType{
			player.Types().EntityType,
			startergrant.Types().EntityType,
			collection.Types().EntityType,
			unlock.Types().EntityType,
			deck.Types().EntityType,
		},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			return g.grants.Handle(ctx, event.Event)
		},
	})
}

// 対戦のイベントの通知を、その台でつないでいる Subscription（対戦の進み）に配る。投影・ポリシーと違い処理の記録を持たない
// （取りこぼしは配り役が間隔ごとに DB から読み直して埋める）。名前は購読者の一覧に載せない
func (g *Game) RegisterWatchers(hub *eventdriven.Hub) error {
	return hub.Watch(match.Types().EntityType, func(ref eventdriven.EventRef) {
		id, err := match.NewID(ref.EntityID)
		if err != nil {
			return
		}
		g.matchProgress.Notify(id, ref.AggregateVersion)
	})
}

// つないでいる Subscription をすべて閉じる。HTTP を止めるとき先に呼ぶ（つなぎっぱなしの接続でシャットダウンの時間を使い切らないため）
func (g *Game) CloseSubscriptions() {
	g.matchProgress.Close()
}

// players の投影を 2 つに分けるのは、players の行を 2 つの集約（player・starter_grant）から書くため。それぞれ自分の列だけを書き、
// 行が無ければ作るので、別の集約の間でどちらが先に動いても同じ行になる。名前と購読は internal/wiring の購読者の一覧と照合される
func (g *Game) RegisterProjections(hub *eventdriven.Hub) error {
	players := repository.NewPlayerWriter()
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Player]{
		Name: "players",
		Subscriptions: []domain.EntityType{
			player.Types().EntityType,
		},
		Upsert: players.Upsert,
		Delete: players.Delete,
	}); err != nil {
		return err
	}
	playerStarterGrants := repository.NewPlayerStarterGrantWriter(g.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.PlayerStarterGrant]{
		Name: "player_starter_grants",
		Subscriptions: []domain.EntityType{
			startergrant.Types().EntityType,
		},
		Upsert: playerStarterGrants.Upsert,
		Delete: playerStarterGrants.Delete,
	}); err != nil {
		return err
	}
	avatars := repository.NewAvatarWriter(g.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Avatar]{
		Name: "avatars",
		Subscriptions: []domain.EntityType{
			avatar.Types().EntityType,
		},
		Upsert: avatars.Upsert,
		Delete: avatars.Delete,
	}); err != nil {
		return err
	}
	decks := repository.NewDeckWriter(g.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Deck]{
		Name: "decks",
		Subscriptions: []domain.EntityType{
			deck.Types().EntityType,
		},
		Upsert: decks.Upsert,
		Delete: decks.Delete,
	}); err != nil {
		return err
	}
	unlocks := repository.NewUnlockWriter(g.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Unlock]{
		Name: "unlocks",
		Subscriptions: []domain.EntityType{
			unlock.Types().EntityType,
		},
		Upsert: unlocks.Upsert,
		Delete: unlocks.Delete,
	}); err != nil {
		return err
	}
	rooms := repository.NewRoomWriter(g.entityRefs)
	return eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Room]{
		Name: "rooms",
		Subscriptions: []domain.EntityType{
			room.Types().EntityType,
		},
		Upsert: rooms.Upsert,
		Delete: rooms.Delete,
	})
}

// db は game 用の DB（core_game）。game の入口の操作は、この DB だけを読み書きする（core#33）
func (g *Game) Mount(mux chi.Router, db *sql.DB) {
	graphql.NewHandler(mux, graphql.NewResolver(g.players, g.playerQuery, g.avatars, g.avatarQuery, g.deckQuery, g.unlockQuery, g.boardQuery, g.npcAvatarQuery, g.gameRuleQuery, g.rooms, g.roomQuery, g.matches, g.matchQuery, g.matchEvents, g.matchProgress), db, g.config.Verifier, g.config.Issuer, g.players)
}

// `app schema game` が書く（入口の presentation の graphql パッケージは studio と名前が重なり、cmd/app で並べて import できないため、
// サービスのパッケージから出す）
func WriteSchema(w io.Writer) {
	graphql.WriteSchema(w)
}

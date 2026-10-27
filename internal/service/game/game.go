// game 用 GraphQL の部品（リゾルバー・入口）
package game

import (
	"context"
	"database/sql"
	"io"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
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
}

type Game struct {
	config      Config
	entityRefs  domain.EntityRefResolver
	players     *command.PlayerHandler
	grants      *command.StarterGrantHandler
	playerQuery *query.PlayerHandler
	avatars     *command.AvatarHandler
	avatarQuery *query.AvatarHandler
	boardQuery  *query.BoardHandler
}

func New(registry *domaindef.Registry, config Config) *Game {
	// DB（timestamptz）はマイクロ秒までしか持たないため、そろえておく（studio と同じ）
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	boardReader := repository.NewBoardReader()
	return &Game{
		config:     config,
		entityRefs: identifier.NewEntityRefResolver(registry),
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
			repository.NewStarterDeckReader().ListPublished,
			command.NewCollectionHandler(
				eventstore.New(registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve),
				identifier.New(registry, collection.NewID),
				constraint.UpdateCollectionPlayer,
				now,
			),
			command.NewDeckHandler(
				eventstore.New(registry, deck.NewID, deck.NewEventType, deck.InitialState, deck.Evolve),
				identifier.New(registry, deck.NewID),
				constraint.UpdateDeckStarter,
				constraint.UpdateDeckName,
				constraint.ReserveDeckQuota,
				rules.FromPublished(repository.NewGameRulesReader().Get),
				now,
			),
			now,
		),
		playerQuery: query.NewPlayerHandler(repository.NewPlayerReader().GetStarterGrantStatus),
		avatars: command.NewAvatarHandler(
			eventstore.New(registry, avatar.NewID, avatar.NewEventType, avatar.InitialState, avatar.Evolve),
			identifier.New(registry, avatar.NewID),
			constraint.UpdateAvatarName,
			constraint.ReserveAvatarQuota,
			rules.FromPublished(repository.NewGameRulesReader().Get),
			now,
		),
		avatarQuery: query.NewAvatarHandler(repository.NewAvatarReader().ListByPlayer),
		boardQuery:  query.NewBoardHandler(boardReader.ListPublished, boardReader.GetPublished),
	}
}

// 名前と購読は internal/wiring の購読者の一覧と照合される（studio の RegisterProjections と同じ形）
func (g *Game) RegisterPolicies(hub *eventdriven.Hub) error {
	return eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{
		Name: "starter_grant",
		Subscriptions: []domain.EntityType{
			player.Types().EntityType,
			startergrant.Types().EntityType,
			collection.Types().EntityType,
			deck.Types().EntityType,
		},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			return g.grants.Handle(ctx, event.Event)
		},
	})
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
	return eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Avatar]{
		Name: "avatars",
		Subscriptions: []domain.EntityType{
			avatar.Types().EntityType,
		},
		Upsert: avatars.Upsert,
		Delete: avatars.Delete,
	})
}

// db は game 用の DB（core_game）。game の入口の操作は、この DB だけを読み書きする（core#33）
func (g *Game) Mount(mux chi.Router, db *sql.DB) {
	graphql.NewHandler(mux, graphql.NewResolver(g.players, g.playerQuery, g.avatars, g.avatarQuery, g.boardQuery), db, g.config.Verifier, g.config.Issuer, g.players)
}

// `app schema game` が書く（入口の presentation の graphql パッケージは studio と名前が重なり、cmd/app で並べて import できないため、
// サービスのパッケージから出す）
func WriteSchema(w io.Writer) {
	graphql.WriteSchema(w)
}

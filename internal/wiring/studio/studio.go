// studio 用 GraphQL の部品（コマンド・問い合わせ・リゾルバー・入口）と、studio の read model の投影
package studio

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/inf/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/uif/graphql"
)

// JWT の検証は外の世界（発行元の JWKS）に触れるため、作ったものを受け取る（テストでは idptest の鍵で作る）
type Config struct {
	Verifier *idp.Verifier
	Issuer   string
}

type Studio struct {
	config    Config
	operators *command.OperatorCommands
	creatures *command.CreatureCommands
	items     *command.ItemCommands
	spells    *command.SpellCommands
	cards     *query.CardList
}

func New(registry *domaindef.Registry, config Config) *Studio {
	// DB（timestamptz）はマイクロ秒までしか持たないため、そろえておく（そろえないと、コマンドが返す日時と、
	// 保存したイベントから作る一覧の日時が食い違う）
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	return &Studio{
		config: config,
		operators: command.NewOperatorCommands(
			eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
			identifier.New(registry, operator.NewID),
			constraint.UpdateOperatorEmail,
			constraint.UpdateOperatorSubject,
			constraint.FindOperatorBySubject,
			constraint.FindUnlinkedOperatorByEmail,
			constraint.OperatorExists,
			now,
		),
		creatures: command.NewCreatureCommands(
			eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
			identifier.New(registry, creature.NewID),
			constraint.UpdateCreatureName,
			now,
		),
		items: command.NewItemCommands(
			eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve),
			identifier.New(registry, item.NewID),
			constraint.UpdateItemName,
			now,
		),
		spells: command.NewSpellCommands(
			eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve),
			identifier.New(registry, spell.NewID),
			constraint.UpdateSpellName,
			now,
		),
		cards: query.NewCardList(repository.ListCards),
	}
}

// 最初のオペレーターを作る権限を持つ人はまだいないため、GraphQL（認可）を通さずにコマンドを直接呼ぶ
func (c *Studio) RegisterInitialOperator(ctx context.Context, db *sql.DB, email string) error {
	registered, err := c.operators.RegisterInitial(rdb.WithDB(ctx, db), email)
	if err != nil {
		return err
	}
	if registered {
		slog.InfoContext(ctx, "registered initial operator", "email", email)
	}
	return nil
}

// 名前と購読は migrations の event_subscribers・event_subscriptions と照合される
func (c *Studio) RegisterProjections(hub *eventdriven.Hub) error {
	return eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.CardList]{
		Name:          "card_list",
		Subscriptions: []domain.EntityType{creature.Types().EntityType, item.Types().EntityType, spell.Types().EntityType},
		Upsert:        repository.UpsertCardList,
		Delete:        repository.DeleteCardList,
	})
}

func (c *Studio) Mount(mux chi.Router, db *sql.DB, version string) {
	graphql.NewHandler(mux, graphql.NewResolver(version, c.creatures, c.items, c.spells, c.cards), db, c.config.Verifier, c.config.Issuer, c.operators)
}

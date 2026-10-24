// studio 用 GraphQL の部品（コマンド・問い合わせ・リゾルバー・入口）と、studio の read model の投影
package studio

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql"
)

// JWT の検証は外の世界（発行元の JWKS）に触れるため、作ったものを受け取る（テストでは idptest の鍵で作る）
type Config struct {
	Verifier *idp.Verifier
	Issuer   string
}

type Studio struct {
	config    Config
	operators *command.OperatorHandler
	creatures *command.CreatureHandler
	items     *command.ItemHandler
	spells    *command.SpellHandler
	// 投影もカードの公開 ID を引くため、コマンドと共有する
	entityRefs      domain.EntityRefResolver
	presetDecks     *command.PresetDeckHandler
	cards           *query.CardHandler
	presetDeckList  *query.PresetDeckHandler
	starterDeckSets *command.StarterDeckSetHandler
	starterDeckSet  *query.StarterDeckSetHandler
}

func New(registry *domaindef.Registry, config Config) *Studio {
	// DB（timestamptz）はマイクロ秒までしか持たないため、そろえておく（そろえないと、コマンドが返す日時と、
	// 保存したイベントから作る一覧の日時が食い違う）
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	entityRefs := identifier.NewEntityRefResolver(registry)
	cardReader := repository.NewCardReader()
	presetDeckReader := repository.NewPresetDeckReader()
	return &Studio{
		config:     config,
		entityRefs: entityRefs,
		operators: command.NewOperatorHandler(
			eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
			identifier.New(registry, operator.NewID),
			constraint.UpdateOperatorEmail,
			constraint.UpdateOperatorSubject,
			constraint.FindOperatorBySubject,
			constraint.FindUnlinkedOperatorByEmail,
			constraint.OperatorExists,
			now,
		),
		creatures: command.NewCreatureHandler(
			eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
			identifier.New(registry, creature.NewID),
			constraint.UpdateCreatureName,
			now,
		),
		items: command.NewItemHandler(
			eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve),
			identifier.New(registry, item.NewID),
			constraint.UpdateItemName,
			now,
		),
		spells: command.NewSpellHandler(
			eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve),
			identifier.New(registry, spell.NewID),
			constraint.UpdateSpellName,
			now,
		),
		presetDecks: command.NewPresetDeckHandler(
			eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve),
			identifier.New(registry, presetdeck.NewID),
			entityRefs,
			constraint.UpdatePresetDeckName,
			constraint.UpdatePresetDeckCardReferences,
			now,
		),
		cards:          query.NewCardHandler(cardReader.List, cardReader.Get),
		presetDeckList: query.NewPresetDeckHandler(presetDeckReader.List, presetDeckReader.Get),
		starterDeckSets: command.NewStarterDeckSetHandler(
			eventstore.New(registry, starterdeckset.NewID, starterdeckset.NewEventType, starterdeckset.InitialState, starterdeckset.Evolve),
			identifier.New(registry, starterdeckset.NewID),
			constraint.FindStarterDeckSet,
			constraint.CreateStarterDeckSet,
			constraint.UpdateStarterDeckSetCardReferences,
			eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve),
			identifier.New(registry, presetdeck.NewID),
			now,
		),
		starterDeckSet: query.NewStarterDeckSetHandler(repository.NewStarterDeckSetReader().Get, presetDeckReader.Revisions),
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
	cards := repository.NewCardWriter()
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Card]{
		Name:          "cards",
		Subscriptions: []domain.EntityType{creature.Types().EntityType, item.Types().EntityType, spell.Types().EntityType},
		Upsert:        cards.Upsert,
		Delete:        cards.Delete,
	}); err != nil {
		return err
	}
	presetDecks := repository.NewPresetDeckWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.PresetDeck]{
		Name:          "preset_decks",
		Subscriptions: []domain.EntityType{presetdeck.Types().EntityType},
		Upsert:        presetDecks.Upsert,
		Delete:        presetDecks.Delete,
	}); err != nil {
		return err
	}
	starterDeckSets := repository.NewStarterDeckSetWriter(c.entityRefs)
	return eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.StarterDeckSet]{
		Name:          "starter_deck_sets",
		Subscriptions: []domain.EntityType{starterdeckset.Types().EntityType},
		Upsert:        starterDeckSets.Upsert,
		Delete:        starterDeckSets.Delete,
	})
}

func (c *Studio) Mount(mux chi.Router, db *sql.DB) {
	graphql.NewHandler(mux, graphql.NewResolver(c.creatures, c.items, c.spells, c.presetDecks, c.cards, c.presetDeckList, c.starterDeckSets, c.starterDeckSet), db, c.config.Verifier, c.config.Issuer, c.operators)
}

// `app schema studio` が書く（入口の presentation の graphql パッケージは game と名前が重なり、cmd/app で並べて import できないため、
// サービスのパッケージから出す）
func WriteSchema(w io.Writer) {
	graphql.WriteSchema(w)
}

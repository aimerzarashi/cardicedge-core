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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
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
	// 公開 ID からカードの種類を引いて、状態の変更を Creature・Item・Spell に振り分ける
	cardStatuses *command.CardHandler
	// 投影もカードの公開 ID を引くため、コマンドと共有する
	entityRefs      domain.EntityRefResolver
	presetDecks     *command.PresetDeckHandler
	cards           *query.CardHandler
	presetDeckList  *query.PresetDeckHandler
	grantDecks      *command.GrantDeckHandler
	grantDeckList   *query.GrantDeckHandler
	boards          *command.BoardHandler
	boardList       *query.BoardHandler
	gameRules       *command.GameRulesHandler
	studioRules     *command.StudioRulesHandler
	studioRulesList *query.StudioRulesHandler
	gameRulesList   *query.GameRulesHandler
	npcAvatars      *command.NPCAvatarHandler
	npcAvatarList   *query.NPCAvatarHandler
	npcDecks        *command.NPCDeckHandler
	npcDeckList     *query.NPCDeckHandler
	starterSets     *command.StarterSetHandler
	starterSetList  *query.StarterSetHandler
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
	grantDeckReader := repository.NewGrantDeckReader()
	boardReader := repository.NewBoardReader()
	npcAvatarReader := repository.NewNPCAvatarReader()
	npcDeckReader := repository.NewNPCDeckReader()
	starterSetReader := repository.NewStarterSetReader()
	creatureStore := eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve)
	itemStore := eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve)
	spellStore := eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve)
	creatures := command.NewCreatureHandler(creatureStore, identifier.New(registry, creature.NewID), constraint.UpdateCreatureName, now)
	items := command.NewItemHandler(itemStore, identifier.New(registry, item.NewID), constraint.UpdateItemName, now)
	spells := command.NewSpellHandler(spellStore, identifier.New(registry, spell.NewID), constraint.UpdateSpellName, now)
	gameRules := command.NewGameRulesHandler(
		eventstore.New(registry, gamerules.NewID, gamerules.NewEventType, gamerules.InitialState, gamerules.Evolve),
		identifier.New(registry, gamerules.NewID),
		constraint.FindGameRules,
		constraint.CreateGameRules,
		now,
	)
	studioRules := command.NewStudioRulesHandler(
		eventstore.New(registry, studiorules.NewID, studiorules.NewEventType, studiorules.InitialState, studiorules.Evolve),
		identifier.New(registry, studiorules.NewID),
		constraint.FindStudioRules,
		constraint.CreateStudioRules,
		now,
	)
	return &Studio{
		config:          config,
		gameRules:       gameRules,
		studioRules:     studioRules,
		studioRulesList: query.NewStudioRulesHandler(repository.NewStudioRulesReader().Get),
		gameRulesList:   query.NewGameRulesHandler(repository.NewGameRulesReader().Get),
		entityRefs:      entityRefs,
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
		creatures:    creatures,
		items:        items,
		spells:       spells,
		cardStatuses: command.NewCardHandler(entityRefs, creatures, items, spells),
		presetDecks: command.NewPresetDeckHandler(
			eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve),
			identifier.New(registry, presetdeck.NewID),
			entityRefs,
			constraint.UpdatePresetDeckName,
			constraint.UpdatePresetDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			gameRules.Get,
			now,
		),
		cards:          query.NewCardHandler(cardReader.List, cardReader.Get),
		presetDeckList: query.NewPresetDeckHandler(presetDeckReader.List, presetDeckReader.Get),
		grantDecks: command.NewGrantDeckHandler(
			eventstore.New(registry, grantdeck.NewID, grantdeck.NewEventType, grantdeck.InitialState, grantdeck.Evolve),
			identifier.New(registry, grantdeck.NewID),
			entityRefs,
			constraint.UpdateGrantDeckName,
			constraint.UpdateGrantDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve),
			identifier.New(registry, presetdeck.NewID),
			gameRules.Get,
			now,
		),
		grantDeckList: query.NewGrantDeckHandler(grantDeckReader.List, grantDeckReader.Get, presetDeckReader.Revisions),
		boards: command.NewBoardHandler(
			eventstore.New(registry, board.NewID, board.NewEventType, board.InitialState, board.Evolve),
			identifier.New(registry, board.NewID),
			constraint.UpdateBoardName,
			studioRules.Get,
			now,
		),
		boardList: query.NewBoardHandler(boardReader.List, boardReader.Get),
		npcAvatars: command.NewNPCAvatarHandler(
			eventstore.New(registry, npcavatar.NewID, npcavatar.NewEventType, npcavatar.InitialState, npcavatar.Evolve),
			identifier.New(registry, npcavatar.NewID),
			entityRefs,
			constraint.UpdateNPCAvatarName,
			constraint.UpdateNPCAvatarDeckReferences,
			command.NewNPCDeckStatusReader(eventstore.New(registry, npcdeck.NewID, npcdeck.NewEventType, npcdeck.InitialState, npcdeck.Evolve)).Statuses,
			now,
		),
		npcAvatarList: query.NewNPCAvatarHandler(npcAvatarReader.List, npcAvatarReader.Get),
		npcDecks: command.NewNPCDeckHandler(
			eventstore.New(registry, npcdeck.NewID, npcdeck.NewEventType, npcdeck.InitialState, npcdeck.Evolve),
			identifier.New(registry, npcdeck.NewID),
			entityRefs,
			constraint.UpdateNPCDeckName,
			constraint.UpdateNPCDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve),
			identifier.New(registry, presetdeck.NewID),
			gameRules.Get,
			now,
		),
		npcDeckList: query.NewNPCDeckHandler(npcDeckReader.List, npcDeckReader.Get, presetDeckReader.Revisions),
		starterSets: command.NewStarterSetHandler(
			eventstore.New(registry, starterset.NewID, starterset.NewEventType, starterset.InitialState, starterset.Evolve),
			identifier.New(registry, starterset.NewID),
			entityRefs,
			constraint.UpdateStarterSetName,
			constraint.UpdateStarterSetGrantDeckReferences,
			constraint.UpdateStarterSetBoardReferences,
			constraint.UpdateStarterSetNPCAvatarReferences,
			command.NewGrantDeckStatusReader(eventstore.New(registry, grantdeck.NewID, grantdeck.NewEventType, grantdeck.InitialState, grantdeck.Evolve)).Statuses,
			command.NewBoardStatusReader(eventstore.New(registry, board.NewID, board.NewEventType, board.InitialState, board.Evolve)).Statuses,
			command.NewNPCAvatarStatusReader(eventstore.New(registry, npcavatar.NewID, npcavatar.NewEventType, npcavatar.InitialState, npcavatar.Evolve)).Statuses,
			constraint.FindGameRules,
			constraint.ReserveStarterSetQuota,
			constraint.ReleaseStarterSetQuota,
			now,
		),
		starterSetList: query.NewStarterSetHandler(starterSetReader.List, starterSetReader.Get),
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

// 起動時に、ゲーム全体のルールが無ければ既定の値で作る。複数台で先を越されたら「ある」とみなす（初期オペレーターと同じ）。
// 初めて使うときに作る案は、読むだけの処理にも「無ければ作る」と同時の扱いが入り込むため採らなかった
func (c *Studio) CreateInitialGameRules(ctx context.Context, db *sql.DB) error {
	created, err := c.gameRules.CreateInitial(rdb.WithDB(ctx, db))
	if err != nil {
		return err
	}
	if created {
		slog.InfoContext(ctx, "created game rules")
	}
	return nil
}

// 起動時に、studio_rules が無ければ既定の値で作る（ゲーム全体のルールと同じ）
func (c *Studio) CreateInitialStudioRules(ctx context.Context, db *sql.DB) error {
	created, err := c.studioRules.CreateInitial(rdb.WithDB(ctx, db))
	if err != nil {
		return err
	}
	if created {
		slog.InfoContext(ctx, "created studio rules")
	}
	return nil
}

// 名前と購読は migrations の event_subscribers・event_subscriptions と照合される
func (c *Studio) RegisterProjections(hub *eventdriven.Hub) error {
	cards := repository.NewCardWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Card]{
		Name: "cards",
		Subscriptions: []domain.EntityType{
			creature.Types().EntityType,
			item.Types().EntityType,
			spell.Types().EntityType,
		},
		Upsert: cards.Upsert,
		Delete: cards.Delete,
	}); err != nil {
		return err
	}
	presetDecks := repository.NewPresetDeckWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.PresetDeck]{
		Name: "preset_decks",
		Subscriptions: []domain.EntityType{
			presetdeck.Types().EntityType,
		},
		Upsert: presetDecks.Upsert,
		Delete: presetDecks.Delete,
	}); err != nil {
		return err
	}
	grantDecks := repository.NewGrantDeckWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.GrantDeck]{
		Name: "grant_decks",
		Subscriptions: []domain.EntityType{
			grantdeck.Types().EntityType,
		},
		Upsert: grantDecks.Upsert,
		Delete: grantDecks.Delete,
	}); err != nil {
		return err
	}
	boards := repository.NewBoardWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.Board]{
		Name: "boards",
		Subscriptions: []domain.EntityType{
			board.Types().EntityType,
		},
		Upsert: boards.Upsert,
		Delete: boards.Delete,
	}); err != nil {
		return err
	}
	gameRules := repository.NewGameRulesWriter()
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.GameRules]{
		Name: "game_rules",
		Subscriptions: []domain.EntityType{
			gamerules.Types().EntityType,
		},
		Upsert: gameRules.Upsert,
		Delete: gameRules.Delete,
	}); err != nil {
		return err
	}
	studioRules := repository.NewStudioRulesWriter()
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.StudioRules]{
		Name: "studio_rules",
		Subscriptions: []domain.EntityType{
			studiorules.Types().EntityType,
		},
		Upsert: studioRules.Upsert,
		Delete: studioRules.Delete,
	}); err != nil {
		return err
	}
	npcAvatars := repository.NewNPCAvatarWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.NPCAvatar]{
		Name: "npc_avatars",
		Subscriptions: []domain.EntityType{
			npcavatar.Types().EntityType,
		},
		Upsert: npcAvatars.Upsert,
		Delete: npcAvatars.Delete,
	}); err != nil {
		return err
	}
	npcDecks := repository.NewNPCDeckWriter(c.entityRefs)
	if err := eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.NPCDeck]{
		Name: "npc_decks",
		Subscriptions: []domain.EntityType{
			npcdeck.Types().EntityType,
		},
		Upsert: npcDecks.Upsert,
		Delete: npcDecks.Delete,
	}); err != nil {
		return err
	}
	starterSets := repository.NewStarterSetWriter(c.entityRefs)
	return eventdriven.RegisterProjection(hub, eventdriven.ProjectionSpec[readmodel.StarterSet]{
		Name: "starter_sets",
		Subscriptions: []domain.EntityType{
			starterset.Types().EntityType,
		},
		Upsert: starterSets.Upsert,
		Delete: starterSets.Delete,
	})
}

func (c *Studio) Mount(mux chi.Router, db *sql.DB) {
	graphql.NewHandler(mux, graphql.NewResolver(c.creatures, c.items, c.spells, c.cardStatuses, c.presetDecks, c.cards, c.presetDeckList, c.grantDecks, c.grantDeckList, c.boards, c.boardList, c.studioRulesList, c.gameRulesList, c.npcAvatars, c.npcAvatarList, c.npcDecks, c.npcDeckList, c.starterSets, c.starterSetList), db, c.config.Verifier, c.config.Issuer, c.operators)
}

// `app schema studio` が書く（入口の presentation の graphql パッケージは game と名前が重なり、cmd/app で並べて import できないため、
// サービスのパッケージから出す）
func WriteSchema(w io.Writer) {
	graphql.WriteSchema(w)
}

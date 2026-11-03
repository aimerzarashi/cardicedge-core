package graphql

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
)

// 実装は command.CreatureHandler
type CreatureRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredCreature, error)
}

// 実装は command.ItemHandler
type ItemRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredItem, error)
}

// 実装は command.SpellHandler
type SpellRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredSpell, error)
}

// 実装は command.CardHandler
type CardStatusHandler interface {
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedCard, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedCard, error)
}

// 実装は command.PresetDeckHandler
type PresetDeckHandler interface {
	Register(ctx context.Context, name string) (command.ChangedPresetDeck, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedPresetDeck, error)
	SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []command.CardCountInput) (command.ChangedPresetDeck, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
}

// 実装は query.CardHandler
type CardLister interface {
	CardGetter
	List(ctx context.Context, filter query.CardFilter, first *int, after *string) (pagination.Connection[query.Card], error)
}

// 実装は query.PresetDeckHandler
type PresetDeckLister interface {
	List(ctx context.Context, filter query.PresetDeckFilter, first *int, after *string) (pagination.Connection[query.PresetDeck], error)
	Get(ctx context.Context, id uuid.UUID) (query.PresetDeck, error)
}

// 実装は command.GrantDeckHandler
type GrantDeckHandler interface {
	Register(ctx context.Context, name string) (command.ChangedGrantDeck, error)
	RegisterFromPresetDeck(ctx context.Context, presetDeckID uuid.UUID, name string) (command.ChangedGrantDeck, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedGrantDeck, error)
	SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []command.CardCountInput) (command.ChangedGrantDeck, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedGrantDeck, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedGrantDeck, error)
}

// 実装は query.GrantDeckHandler
type GrantDeckLister interface {
	List(ctx context.Context, filter query.GrantDeckFilter, first *int, after *string) (pagination.Connection[query.GrantDeck], error)
	Get(ctx context.Context, id uuid.UUID) (query.GrantDeck, error)
}

// 実装は command.BoardHandler
type BoardHandler interface {
	Register(ctx context.Context, name string) (command.ChangedBoard, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedBoard, error)
	ChangeLayout(ctx context.Context, id uuid.UUID, revision int64, layout command.BoardLayoutInput) (command.ChangedBoard, error)
	CheckLayout(ctx context.Context, layout command.BoardLayoutInput) ([]command.BoardPublishProblem, error)
	ChangeDefaultRules(ctx context.Context, id uuid.UUID, revision int64, rules command.BoardDefaultRulesInput) (command.ChangedBoard, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedBoard, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedBoard, error)
}

// 実装は query.BoardHandler
type BoardLister interface {
	List(ctx context.Context, filter query.BoardFilter, first *int, after *string) (pagination.Connection[query.Board], error)
	Get(ctx context.Context, id uuid.UUID) (query.Board, error)
}

// 実装は command.NPCAvatarHandler
type NPCAvatarHandler interface {
	Register(ctx context.Context, name string) (command.ChangedNPCAvatar, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedNPCAvatar, error)
	SetDecks(ctx context.Context, id uuid.UUID, revision int64, deckIDs []uuid.UUID) (command.ChangedNPCAvatar, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedNPCAvatar, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedNPCAvatar, error)
}

// 実装は query.NPCAvatarHandler
type NPCAvatarLister interface {
	List(ctx context.Context, filter query.NPCAvatarFilter, first *int, after *string) (pagination.Connection[query.NPCAvatar], error)
	Get(ctx context.Context, id uuid.UUID) (query.NPCAvatar, error)
}

// 実装は command.NPCDeckHandler
type NPCDeckHandler interface {
	Register(ctx context.Context, name string) (command.ChangedNPCDeck, error)
	RegisterFromPresetDeck(ctx context.Context, presetDeckID uuid.UUID, name string) (command.ChangedNPCDeck, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedNPCDeck, error)
	SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []command.CardCountInput) (command.ChangedNPCDeck, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedNPCDeck, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedNPCDeck, error)
}

// 実装は query.NPCDeckHandler
type NPCDeckLister interface {
	List(ctx context.Context, filter query.NPCDeckFilter, first *int, after *string) (pagination.Connection[query.NPCDeck], error)
	Get(ctx context.Context, id uuid.UUID) (query.NPCDeck, error)
}

// 実装は query.StudioRulesHandler
type StudioRulesGetter interface {
	Get(ctx context.Context) (query.StudioRules, error)
}

// 実装は query.GameRulesHandler
type GameRulesGetter interface {
	Get(ctx context.Context) (query.GameRules, error)
}

type Resolver struct {
	creatures      CreatureRegistrar
	items          ItemRegistrar
	spells         SpellRegistrar
	cardStatuses   CardStatusHandler
	presetDecks    PresetDeckHandler
	cards          CardLister
	presetDeckList PresetDeckLister
	grantDecks     GrantDeckHandler
	grantDeckList  GrantDeckLister
	boards         BoardHandler
	boardList      BoardLister
	studioRules    StudioRulesGetter
	gameRules      GameRulesGetter
	npcAvatars     NPCAvatarHandler
	npcAvatarList  NPCAvatarLister
	npcDecks       NPCDeckHandler
	npcDeckList    NPCDeckLister
}

func NewResolver(
	creatures CreatureRegistrar,
	items ItemRegistrar,
	spells SpellRegistrar,
	cardStatuses CardStatusHandler,
	presetDecks PresetDeckHandler,
	cards CardLister,
	presetDeckList PresetDeckLister,
	grantDecks GrantDeckHandler,
	grantDeckList GrantDeckLister,
	boards BoardHandler,
	boardList BoardLister,
	studioRules StudioRulesGetter,
	gameRules GameRulesGetter,
	npcAvatars NPCAvatarHandler,
	npcAvatarList NPCAvatarLister,
	npcDecks NPCDeckHandler,
	npcDeckList NPCDeckLister,
) *Resolver {
	return &Resolver{
		creatures: creatures, items: items, spells: spells, cardStatuses: cardStatuses, presetDecks: presetDecks,
		cards: cards, presetDeckList: presetDeckList, grantDecks: grantDecks, grantDeckList: grantDeckList,
		boards: boards, boardList: boardList, studioRules: studioRules, gameRules: gameRules, npcAvatars: npcAvatars, npcAvatarList: npcAvatarList,
		npcDecks: npcDecks, npcDeckList: npcDeckList,
	}
}

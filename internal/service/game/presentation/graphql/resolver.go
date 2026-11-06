package graphql

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 実装は command.PlayerHandler
type Players interface {
	Register(ctx context.Context) (command.RegisteredPlayer, error)
	Me(ctx context.Context) (command.RegisteredPlayer, error)
}

// 実装は query.PlayerHandler
type PlayerQueries interface {
	StarterGrant(ctx context.Context, playerID uuid.UUID) (query.StarterGrantStatus, error)
}

// 実装は command.AvatarHandler
type Avatars interface {
	Register(ctx context.Context, name string) (command.RegisteredAvatar, error)
}

// 実装は query.AvatarHandler
type AvatarQueries interface {
	ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Avatar, error)
}

// 実装は query.BoardHandler
type BoardQueries interface {
	List(ctx context.Context, first *int, after *string) (pagination.Connection[query.Board], error)
	Get(ctx context.Context, id uuid.UUID) (query.Board, error)
}

// 実装は query.UnlockHandler
type UnlockQueries interface {
	ListBoardsByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Board, error)
}

// 実装は query.DeckHandler
type DeckQueries interface {
	ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Deck, error)
	Get(ctx context.Context, playerID uuid.UUID, deckID uuid.UUID) (query.Deck, error)
}

// 実装は query.NPCAvatarHandler
type NPCAvatarQueries interface {
	ListUnlockedByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.NPCAvatar, error)
	GetUnlocked(ctx context.Context, playerID uuid.UUID, id uuid.UUID) (query.NPCAvatar, error)
}

// 実装は command.RoomHandler
type Rooms interface {
	Create(ctx context.Context, name string) (command.ChangedRoom, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedRoom, error)
	ChangeSeats(ctx context.Context, id uuid.UUID, revision int64, seats []command.RoomSeatKind) (command.ChangedRoom, error)
	ChangeMatchRules(ctx context.Context, id uuid.UUID, revision int64, rules command.RoomMatchRulesInput) (command.ChangedRoom, error)
	ChangeSeatChoice(ctx context.Context, id uuid.UUID, revision int64, seat int, avatarID uuid.UUID, deckID uuid.UUID) (command.ChangedRoom, error)
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedRoom, error)
	DeclareReady(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedRoom, error)
	DeclareNext(ctx context.Context, id uuid.UUID, revision int64, continues bool) (command.ChangedRoom, error)
	Resume(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedRoom, error)
	Close(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedRoom, error)
}

// 実装は query.RoomHandler
type RoomQueries interface {
	ListByPlayer(ctx context.Context, playerID uuid.UUID, first *int, after *string) (pagination.Connection[query.Room], error)
	Get(ctx context.Context, playerID uuid.UUID, roomID uuid.UUID) (query.Room, error)
}

// 実装は command.MatchHandler
type Matches interface {
	Draw(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedMatch, error)
	Discard(ctx context.Context, id uuid.UUID, revision int64, handIndex int) (command.ChangedMatch, error)
	Roll(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedMatch, error)
	ChoosePath(ctx context.Context, id uuid.UUID, revision int64, x, y int) (command.ChangedMatch, error)
	EndTurn(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedMatch, error)
	ChooseRewardPack(ctx context.Context, id uuid.UUID, revision int64, pack string) (command.ChangedMatch, error)
}

// 実装は query.MatchHandler
type MatchQueries interface {
	Get(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (query.Match, error)
}

// 実装は query.MatchEventsHandler
type MatchEventQueries interface {
	ListAfter(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID, after int64) ([]query.MatchEvent, error)
}

// 実装は query.MatchProgressBroker
type MatchProgressSubscriptions interface {
	Subscribe(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (<-chan query.MatchProgress, error)
}

// 実装は query.GameRulesHandler
type GameRulesQueries interface {
	Get(ctx context.Context) (query.GameRules, error)
}

type Resolver struct {
	players        Players
	playerQuery    PlayerQueries
	avatars        Avatars
	avatarQuery    AvatarQueries
	deckQuery      DeckQueries
	unlockQuery    UnlockQueries
	boardQuery     BoardQueries
	npcAvatarQuery NPCAvatarQueries
	gameRuleQuery  GameRulesQueries
	rooms          Rooms
	roomQuery      RoomQueries
	matches        Matches
	matchQuery     MatchQueries
	matchEvents    MatchEventQueries
	matchProgress  MatchProgressSubscriptions
}

func NewResolver(players Players, playerQuery PlayerQueries, avatars Avatars, avatarQuery AvatarQueries, deckQuery DeckQueries, unlockQuery UnlockQueries, boardQuery BoardQueries, npcAvatarQuery NPCAvatarQueries, gameRuleQuery GameRulesQueries, rooms Rooms, roomQuery RoomQueries, matches Matches, matchQuery MatchQueries, matchEvents MatchEventQueries, matchProgress MatchProgressSubscriptions) *Resolver {
	return &Resolver{players: players, playerQuery: playerQuery, avatars: avatars, avatarQuery: avatarQuery, deckQuery: deckQuery, unlockQuery: unlockQuery, boardQuery: boardQuery, npcAvatarQuery: npcAvatarQuery, gameRuleQuery: gameRuleQuery, rooms: rooms, roomQuery: roomQuery, matches: matches, matchQuery: matchQuery, matchEvents: matchEvents, matchProgress: matchProgress}
}

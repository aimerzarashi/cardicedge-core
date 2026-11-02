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

// 実装は query.NPCAvatarHandler
type NPCAvatarQueries interface {
	List(ctx context.Context, first *int, after *string) (pagination.Connection[query.NPCAvatar], error)
	Get(ctx context.Context, id uuid.UUID) (query.NPCAvatar, error)
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
	boardQuery     BoardQueries
	npcAvatarQuery NPCAvatarQueries
	gameRuleQuery  GameRulesQueries
}

func NewResolver(players Players, playerQuery PlayerQueries, avatars Avatars, avatarQuery AvatarQueries, boardQuery BoardQueries, npcAvatarQuery NPCAvatarQueries, gameRuleQuery GameRulesQueries) *Resolver {
	return &Resolver{players: players, playerQuery: playerQuery, avatars: avatars, avatarQuery: avatarQuery, boardQuery: boardQuery, npcAvatarQuery: npcAvatarQuery, gameRuleQuery: gameRuleQuery}
}

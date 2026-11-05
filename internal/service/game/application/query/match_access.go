package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// 対戦を読んでよいプレイヤーか確かめ、対戦の内部の ID を返す。参加していない対戦も、無いのと同じ ErrMatchNotFound
type AuthorizeMatchFunc func(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, error)

// 対戦の公開 ID から、見ているプレイヤーが参加している対戦だけを開く（状態・進み・差分の読み出しで同じ判定にするため 1 か所にまとめる）
type MatchAccess struct {
	eventStore domain.EventStore[match.ID, match.Event, match.State]
	matches    domain.Identifier[match.ID]
	players    domain.Identifier[player.ID]
}

func NewMatchAccess(
	eventStore domain.EventStore[match.ID, match.Event, match.State],
	matches domain.Identifier[match.ID],
	players domain.Identifier[player.ID],
) *MatchAccess {
	return &MatchAccess{eventStore: eventStore, matches: matches, players: players}
}

func (a *MatchAccess) Open(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, match.Match, error) {
	id, err := a.matches.Resolve(ctx, matchID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return 0, match.Match{}, ErrMatchNotFound
	}
	if err != nil {
		return 0, match.Match{}, err
	}
	viewer, err := a.players.Resolve(ctx, playerID)
	if err != nil {
		return 0, match.Match{}, err
	}
	state, err := a.eventStore.Load(ctx, id)
	if err != nil {
		return 0, match.Match{}, err
	}
	current, ok := state.(match.Match)
	if !ok || !current.HasPlayer(viewer) {
		return 0, match.Match{}, ErrMatchNotFound
	}
	return id, current, nil
}

// AuthorizeMatchFunc として渡す
func (a *MatchAccess) Authorize(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, error) {
	id, _, err := a.Open(ctx, playerID, matchID)
	return id, err
}

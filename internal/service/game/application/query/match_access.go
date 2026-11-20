package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// 対戦を読んでよいプレイヤーか確かめ、対戦の内部の ID を返す。読めない対戦も、無いのと同じ ErrMatchNotFound
type AuthorizeMatchFunc func(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, error)

// その人が作った部屋で始めた対戦か（read model の rooms で引く）
type OwnsRoomOfMatchFunc func(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (bool, error)

// 対戦の公開 ID から、見ているプレイヤーが読める対戦だけを開く（状態・進み・差分の読み出しで同じ判定にするため 1 か所にまとめる）。
// 読めるのは参加者と、その対戦を始めた部屋を作った人（全員 NPC の部屋で、作った人は参加せずに見るため。2026-11-17 オーナー。core#179）
type MatchAccess struct {
	eventStore      domain.EventStore[match.ID, match.Event, match.State]
	matches         domain.Identifier[match.ID]
	players         domain.Identifier[player.ID]
	ownsRoomOfMatch OwnsRoomOfMatchFunc
}

func NewMatchAccess(
	eventStore domain.EventStore[match.ID, match.Event, match.State],
	matches domain.Identifier[match.ID],
	players domain.Identifier[player.ID],
	ownsRoomOfMatch OwnsRoomOfMatchFunc,
) *MatchAccess {
	return &MatchAccess{eventStore: eventStore, matches: matches, players: players, ownsRoomOfMatch: ownsRoomOfMatch}
}

// 見ているプレイヤーの内部の ID も返す（参加者の何番目かを割り出して、その人に見せるものを決めるため）
func (a *MatchAccess) Open(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, match.Match, player.ID, error) {
	id, err := a.matches.Resolve(ctx, matchID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return 0, match.Match{}, 0, ErrMatchNotFound
	}
	if err != nil {
		return 0, match.Match{}, 0, err
	}
	viewer, err := a.players.Resolve(ctx, playerID)
	if err != nil {
		return 0, match.Match{}, 0, err
	}
	state, err := a.eventStore.Load(ctx, id)
	if err != nil {
		return 0, match.Match{}, 0, err
	}
	current, ok := state.(match.Match)
	if !ok {
		return 0, match.Match{}, 0, ErrMatchNotFound
	}
	if !current.HasPlayer(viewer) {
		owns, err := a.ownsRoomOfMatch(ctx, playerID, matchID)
		if err != nil {
			return 0, match.Match{}, 0, err
		}
		if !owns {
			return 0, match.Match{}, 0, ErrMatchNotFound
		}
	}
	return id, current, viewer, nil
}

// AuthorizeMatchFunc として渡す
func (a *MatchAccess) Authorize(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, error) {
	id, _, _, err := a.Open(ctx, playerID, matchID)
	return id, err
}

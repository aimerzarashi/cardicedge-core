package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// game_rules を読む（表は 1 行）
type GameRulesReader struct{}

func NewGameRulesReader() *GameRulesReader {
	return &GameRulesReader{}
}

// 行が無ければ query.ErrGameRulesNotProjected
func (r *GameRulesReader) Get(ctx context.Context) (query.GameRules, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.GameRules{}, err
	}
	row, err := models.GameRules().One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.GameRules{}, query.ErrGameRulesNotProjected
	}
	if err != nil {
		return query.GameRules{}, err
	}
	var metadata readmodel.GameRulesMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return query.GameRules{}, fmt.Errorf("game_rules.metadata of %s: %w", row.GameRulesID, err)
	}
	match := metadata.Rules.Match
	matchRules := query.MatchRules{MinPlayers: match.MinPlayers.Int(), MaxPlayers: match.MaxPlayers.Int(), MaxHandCards: match.MaxHandCards.Int()}
	return query.GameRules{Match: matchRules}, nil
}

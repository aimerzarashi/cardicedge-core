package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// core_game の game_rules（make publish-master が studio の read model から写した表）を読む
type GameRulesReader struct{}

func NewGameRulesReader() *GameRulesReader {
	return &GameRulesReader{}
}

// 写した表の metadata のうち、値の項目。studio の read model の型を import せず、この JSON の形だけを契約にする
// （値は gamerules.Rules をそのまま持つので、項目を足しても読み方は変わらない）
type gameRulesMetadata struct {
	Rules gamerules.Rules `json:"rules"`
}

// 行はゲーム全体で 1 行（写されていなければ無い）なので、条件を付けずに 1 行読む。
// 値は読むときにも確かめる（写した行が壊れていて、上限が 0 などで動くのを防ぐため）
func (r *GameRulesReader) Get(ctx context.Context) (gamerules.Rules, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return gamerules.Rules{}, false, err
	}
	row, err := models.GameRules().One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return gamerules.Rules{}, false, nil
	}
	if err != nil {
		return gamerules.Rules{}, false, err
	}
	var metadata gameRulesMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return gamerules.Rules{}, false, fmt.Errorf("game_rules.metadata of %s: %w", row.GameRulesID, err)
	}
	rules, err := gamerules.New(metadata.Rules.Player, metadata.Rules.Deck, metadata.Rules.StarterDeck, metadata.Rules.Match)
	if err != nil {
		return gamerules.Rules{}, false, fmt.Errorf("game_rules.metadata of %s: %w", row.GameRulesID, err)
	}
	return rules, true, nil
}

package repository

import (
	"context"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

type GameRulesWriter struct{}

func NewGameRulesWriter() *GameRulesWriter {
	return &GameRulesWriter{}
}

func (w *GameRulesWriter) Upsert(ctx context.Context, gameRules readmodel.GameRules) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(readmodel.GameRulesMetadata{
		ID: gameRules.GameRulesID, Revision: gameRules.Revision, RegisteredAt: gameRules.RegisteredAt, UpdatedAt: gameRules.UpdatedAt, Rules: gameRules.Rules,
	})
	if err != nil {
		return err
	}
	row := &models.GameRule{GameRulesID: gameRules.GameRulesID.String(), Metadata: raw}
	return row.Upsert(ctx, exec, true,
		[]string{models.GameRuleColumns.GameRulesID},
		boil.Whitelist(models.GameRuleColumns.Metadata),
		boil.Infer(),
	)
}

// 削除のイベントは無いため呼ばれない（投影の宣言に要るので置く）
func (w *GameRulesWriter) Delete(ctx context.Context, gameRules readmodel.GameRules) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.GameRules(models.GameRuleWhere.GameRulesID.EQ(gameRules.GameRulesID.String())).DeleteAll(ctx, exec)
	return err
}

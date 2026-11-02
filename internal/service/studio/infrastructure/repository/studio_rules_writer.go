package repository

import (
	"context"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

type StudioRulesWriter struct{}

func NewStudioRulesWriter() *StudioRulesWriter {
	return &StudioRulesWriter{}
}

func (w *StudioRulesWriter) Upsert(ctx context.Context, studioRules readmodel.StudioRules) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(readmodel.StudioRulesMetadata{
		ID: studioRules.StudioRulesID, Revision: studioRules.Revision, RegisteredAt: studioRules.RegisteredAt, UpdatedAt: studioRules.UpdatedAt, Rules: studioRules.Rules,
	})
	if err != nil {
		return err
	}
	row := &models.StudioRule{StudioRulesID: studioRules.StudioRulesID.String(), Metadata: raw}
	return row.Upsert(ctx, exec, true,
		[]string{models.StudioRuleColumns.StudioRulesID},
		boil.Whitelist(models.StudioRuleColumns.Metadata),
		boil.Infer(),
	)
}

// 削除のイベントは無いため呼ばれない（投影の宣言に要るので置く）
func (w *StudioRulesWriter) Delete(ctx context.Context, studioRules readmodel.StudioRules) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.StudioRules(models.StudioRuleWhere.StudioRulesID.EQ(studioRules.StudioRulesID.String())).DeleteAll(ctx, exec)
	return err
}

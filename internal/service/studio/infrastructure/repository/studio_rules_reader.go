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

// studio_rules を読む（表は 1 行）
type StudioRulesReader struct{}

func NewStudioRulesReader() *StudioRulesReader {
	return &StudioRulesReader{}
}

// 行が無ければ query.ErrStudioRulesNotProjected
func (r *StudioRulesReader) Get(ctx context.Context) (query.StudioRules, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.StudioRules{}, err
	}
	row, err := models.StudioRules().One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.StudioRules{}, query.ErrStudioRulesNotProjected
	}
	if err != nil {
		return query.StudioRules{}, err
	}
	var metadata readmodel.StudioRulesMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return query.StudioRules{}, fmt.Errorf("studio_rules.metadata of %s: %w", row.StudioRulesID, err)
	}
	board := query.BoardRules{MinSize: metadata.Rules.Board.MinSize.Int(), MaxSize: metadata.Rules.Board.MaxSize.Int()}
	return query.StudioRules{Board: board}, nil
}

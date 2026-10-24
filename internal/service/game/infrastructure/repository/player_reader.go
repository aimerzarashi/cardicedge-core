package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

type PlayerReader struct{}

func NewPlayerReader() *PlayerReader {
	return &PlayerReader{}
}

// 行が無い、または metadata に配布の状態が無ければ found は false
func (r *PlayerReader) GetStarterGrantStatus(ctx context.Context, playerID uuid.UUID) (query.StarterGrantStatus, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return "", false, err
	}
	var status sql.NullString
	err = exec.QueryRowContext(ctx, `SELECT metadata->>'starter_grant' FROM players WHERE player_id = $1`, playerID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !status.Valid) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	switch status.String {
	case string(readmodel.StarterGrantInProgress):
		return query.StarterGrantInProgress, true, nil
	case string(readmodel.StarterGrantCompleted):
		return query.StarterGrantCompleted, true, nil
	default:
		return "", false, fmt.Errorf("players.metadata.starter_grant of %s: unknown value %q", playerID, status.String)
	}
}

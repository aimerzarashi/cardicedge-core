package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// starter_sets を読む（一覧と詳細）。並び順は索引（sort_name, starter_set_id）と同じにし、続きは行の比べ方で取る。
// sqlboiler を使わない理由は PresetDeckReader.List と同じ
type StarterSetReader struct{}

func NewStarterSetReader() *StarterSetReader {
	return &StarterSetReader{}
}

func (r *StarterSetReader) List(ctx context.Context, condition query.ListStarterSetsCondition) ([]query.StarterSetListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var where []string
	var args []any
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if condition.After != nil {
		where = append(where, fmt.Sprintf("(sort_name, starter_set_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.StarterSetID)))
	}
	if len(condition.Statuses) > 0 {
		statuses := make([]string, len(condition.Statuses))
		for i, status := range condition.Statuses {
			statuses[i] = string(status)
		}
		where = append(where, fmt.Sprintf("status = ANY(%s)", arg(pq.Array(statuses))))
	}
	if condition.NameContains != "" {
		where = append(where, fmt.Sprintf(`sort_name LIKE '%%' || %s || '%%' ESCAPE '\'`, arg(likeEscaper.Replace(condition.NameContains))))
	}
	sql := "SELECT starter_set_id, sort_name, metadata FROM starter_sets"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, starter_set_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.StarterSetListRow
	for rows.Next() {
		var (
			id       uuid.UUID
			sortName string
			raw      []byte
		)
		if err := rows.Scan(&id, &sortName, &raw); err != nil {
			return nil, err
		}
		starterSet, err := toStarterSet(id, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.StarterSetListCursor{SortName: sortName, StarterSetID: id}
		result = append(result, query.StarterSetListRow{StarterSet: starterSet, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *StarterSetReader) Get(ctx context.Context, id uuid.UUID) (query.StarterSet, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.StarterSet{}, err
	}
	row, err := models.FindStarterSet(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.StarterSet{}, query.ErrStarterSetNotFound
	}
	if err != nil {
		return query.StarterSet{}, err
	}
	return toStarterSet(id, row.Metadata)
}

func toStarterSet(id uuid.UUID, raw []byte) (query.StarterSet, error) {
	var metadata readmodel.StarterSetMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.StarterSet{}, fmt.Errorf("starter_sets.metadata of %s: %w", id, err)
	}
	return query.StarterSet{
		ID: metadata.ID, Name: metadata.Name, Status: metadata.Status,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, RegisteredBy: metadata.RegisteredBy,
		UpdatedAt: metadata.UpdatedAt, UpdatedBy: metadata.UpdatedBy, GrantDecks: metadata.GrantDecks, UnlockBoards: metadata.UnlockBoards, UnlockNPCAvatars: metadata.UnlockNPCAvatars,
	}, nil
}

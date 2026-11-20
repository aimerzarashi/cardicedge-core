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

// npc_avatars を読む（一覧と詳細）。並び順は索引（sort_name, npc_avatar_id）と同じにし、続きは行の比べ方で取る。
// sqlboiler を使わない理由は PresetDeckReader.List と同じ
type NPCAvatarReader struct{}

func NewNPCAvatarReader() *NPCAvatarReader {
	return &NPCAvatarReader{}
}

func (r *NPCAvatarReader) List(ctx context.Context, condition query.ListNPCAvatarsCondition) ([]query.NPCAvatarListRow, error) {
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
		where = append(where, fmt.Sprintf("(sort_name, npc_avatar_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.NPCAvatarID)))
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
	sql := "SELECT npc_avatar_id, sort_name, metadata FROM npc_avatars"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, npc_avatar_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.NPCAvatarListRow
	for rows.Next() {
		var (
			id       uuid.UUID
			sortName string
			raw      []byte
		)
		if err := rows.Scan(&id, &sortName, &raw); err != nil {
			return nil, err
		}
		npcAvatar, err := toNPCAvatar(id, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.NPCAvatarListCursor{SortName: sortName, NPCAvatarID: id}
		result = append(result, query.NPCAvatarListRow{NPCAvatar: npcAvatar, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *NPCAvatarReader) Get(ctx context.Context, id uuid.UUID) (query.NPCAvatar, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.NPCAvatar{}, err
	}
	row, err := models.FindNPCAvatar(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.NPCAvatar{}, query.ErrNPCAvatarNotFound
	}
	if err != nil {
		return query.NPCAvatar{}, err
	}
	return toNPCAvatar(id, row.Metadata)
}

func toNPCAvatar(id uuid.UUID, raw []byte) (query.NPCAvatar, error) {
	var metadata readmodel.NPCAvatarMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.NPCAvatar{}, fmt.Errorf("npc_avatars.metadata of %s: %w", id, err)
	}
	return query.NPCAvatar{
		ID: metadata.ID, Name: metadata.Name, Status: metadata.Status,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, RegisteredBy: metadata.RegisteredBy,
		UpdatedAt: metadata.UpdatedAt, UpdatedBy: metadata.UpdatedBy, Decks: metadata.Decks, Mindsets: metadata.Mindsets,
	}, nil
}

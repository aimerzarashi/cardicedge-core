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

// npc_decks を読む（一覧と詳細）。並び順は索引（sort_name, npc_deck_id）と同じにし、続きは行の比べ方で取る。
// sqlboiler を使わない理由は PresetDeckReader.List と同じ
type NPCDeckReader struct{}

func NewNPCDeckReader() *NPCDeckReader {
	return &NPCDeckReader{}
}

func (r *NPCDeckReader) List(ctx context.Context, condition query.ListNPCDecksCondition) ([]query.NPCDeckListRow, error) {
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
		where = append(where, fmt.Sprintf("(sort_name, npc_deck_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.NPCDeckID)))
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
	sql := "SELECT npc_deck_id, sort_name, total_count, metadata FROM npc_decks"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, npc_deck_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.NPCDeckListRow
	for rows.Next() {
		var (
			id         uuid.UUID
			sortName   string
			totalCount int
			raw        []byte
		)
		if err := rows.Scan(&id, &sortName, &totalCount, &raw); err != nil {
			return nil, err
		}
		npcDeck, err := toNPCDeck(id, totalCount, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.NPCDeckListCursor{SortName: sortName, NPCDeckID: id}
		result = append(result, query.NPCDeckListRow{NPCDeck: npcDeck, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *NPCDeckReader) Get(ctx context.Context, id uuid.UUID) (query.NPCDeck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.NPCDeck{}, err
	}
	row, err := models.FindNPCDeck(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.NPCDeck{}, query.ErrNPCDeckNotFound
	}
	if err != nil {
		return query.NPCDeck{}, err
	}
	return toNPCDeck(id, row.TotalCount, row.Metadata)
}

func toNPCDeck(id uuid.UUID, totalCount int, raw []byte) (query.NPCDeck, error) {
	var metadata readmodel.NPCDeckMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.NPCDeck{}, fmt.Errorf("npc_decks.metadata of %s: %w", id, err)
	}
	cards := make([]query.PresetDeckCard, 0, len(metadata.Cards))
	for _, card := range metadata.Cards {
		cards = append(cards, query.PresetDeckCard{CardID: card.CardID, Count: card.Count})
	}
	var source *query.NPCDeckSource
	if metadata.Source != nil {
		source = &query.NPCDeckSource{PresetDeckID: metadata.Source.PresetDeckID, Revision: metadata.Source.Revision}
	}
	return query.NPCDeck{
		ID: metadata.ID, Name: metadata.Name, Status: metadata.Status, Cards: cards, TotalCount: totalCount, Source: source,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, RegisteredBy: metadata.RegisteredBy,
		UpdatedAt: metadata.UpdatedAt, UpdatedBy: metadata.UpdatedBy,
	}, nil
}

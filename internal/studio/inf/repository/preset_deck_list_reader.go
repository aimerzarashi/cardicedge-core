package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// 並び順は索引（sort_name, preset_deck_id）と同じにし、続きは行の比べ方で取る（理由は ListCards と同じ）。
// sqlboiler を使わない理由も ListCards と同じ
func ListPresetDecks(ctx context.Context, condition query.ListPresetDecksCondition) ([]query.PresetDeckListRow, error) {
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
		where = append(where, fmt.Sprintf("(sort_name, preset_deck_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.PresetDeckID)))
	}
	if condition.NameContains != "" {
		where = append(where, fmt.Sprintf(`sort_name LIKE '%%' || %s || '%%' ESCAPE '\'`, arg(likeEscaper.Replace(condition.NameContains))))
	}
	sql := "SELECT preset_deck_id, sort_name, total_count, metadata FROM preset_deck_lists"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, preset_deck_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.PresetDeckListRow
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
		deck, err := toPresetDeck(id, totalCount, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.PresetDeckListCursor{SortName: sortName, PresetDeckID: id}
		result = append(result, query.PresetDeckListRow{PresetDeck: deck, Cursor: cursor})
	}
	return result, rows.Err()
}

func GetPresetDeck(ctx context.Context, id uuid.UUID) (query.PresetDeck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.PresetDeck{}, err
	}
	row, err := models.FindPresetDeckList(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.PresetDeck{}, query.ErrPresetDeckNotFound
	}
	if err != nil {
		return query.PresetDeck{}, err
	}
	return toPresetDeck(id, row.TotalCount, row.Metadata)
}

func toPresetDeck(id uuid.UUID, totalCount int, raw []byte) (query.PresetDeck, error) {
	var metadata readmodel.PresetDeckListMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.PresetDeck{}, fmt.Errorf("preset_deck_lists.metadata of %s: %w", id, err)
	}
	cards := make([]query.PresetDeckCard, 0, len(metadata.Cards))
	for _, card := range metadata.Cards {
		cards = append(cards, query.PresetDeckCard{CardID: card.CardID, Count: card.Count})
	}
	return query.PresetDeck{
		ID: metadata.ID, Name: metadata.Name, Cards: cards, TotalCount: totalCount,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, UpdatedAt: metadata.UpdatedAt,
	}, nil
}

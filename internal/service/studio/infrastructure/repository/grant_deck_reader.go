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

// grant_decks を読む（一覧と詳細）。並び順は索引（sort_name, grant_deck_id）と同じにし、続きは行の比べ方で取る。
// sqlboiler を使わない理由は PresetDeckReader.List と同じ
type GrantDeckReader struct{}

func NewGrantDeckReader() *GrantDeckReader {
	return &GrantDeckReader{}
}

func (r *GrantDeckReader) List(ctx context.Context, condition query.ListGrantDecksCondition) ([]query.GrantDeckListRow, error) {
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
		where = append(where, fmt.Sprintf("(sort_name, grant_deck_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.GrantDeckID)))
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
	sql := "SELECT grant_deck_id, sort_name, total_count, metadata FROM grant_decks"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, grant_deck_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.GrantDeckListRow
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
		deck, err := toGrantDeck(id, totalCount, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.GrantDeckListCursor{SortName: sortName, GrantDeckID: id}
		result = append(result, query.GrantDeckListRow{GrantDeck: deck, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *GrantDeckReader) Get(ctx context.Context, id uuid.UUID) (query.GrantDeck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.GrantDeck{}, err
	}
	row, err := models.FindGrantDeck(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.GrantDeck{}, query.ErrGrantDeckNotFound
	}
	if err != nil {
		return query.GrantDeck{}, err
	}
	return toGrantDeck(id, row.TotalCount, row.Metadata)
}

func toGrantDeck(id uuid.UUID, totalCount int, raw []byte) (query.GrantDeck, error) {
	var metadata readmodel.GrantDeckMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.GrantDeck{}, fmt.Errorf("grant_decks.metadata of %s: %w", id, err)
	}
	cards := make([]query.PresetDeckCard, 0, len(metadata.Cards))
	for _, card := range metadata.Cards {
		cards = append(cards, query.PresetDeckCard{CardID: card.CardID, Count: card.Count})
	}
	var source *query.GrantDeckSource
	if metadata.Source != nil {
		source = &query.GrantDeckSource{PresetDeckID: metadata.Source.PresetDeckID, Revision: metadata.Source.Revision}
	}
	return query.GrantDeck{
		ID: metadata.ID, Name: metadata.Name, Status: metadata.Status, Cards: cards, TotalCount: totalCount, Source: source,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, UpdatedAt: metadata.UpdatedAt,
		RegisteredBy: metadata.RegisteredBy, UpdatedBy: metadata.UpdatedBy,
	}, nil
}

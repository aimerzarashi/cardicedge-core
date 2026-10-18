package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// LIKE の記号（% _）とエスケープの文字を、文字どおりに比べるため
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// 並び順は索引（card_type, sort_name, card_id）と同じにし、続きは行の比べ方（(a, b, c) > (x, y, z)）で取る。
// sort_name は列に COLLATE "C" を付けているため、並べ替えと比べ方が OS の照合順序に左右されず一致する
func ListCards(ctx context.Context, condition query.ListCardsCondition) ([]query.CardListRow, error) {
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
		where = append(where, fmt.Sprintf("(card_type, sort_name, card_id) > (%s, %s, %s)",
			arg(int16(condition.After.CardType)), arg(condition.After.SortName), arg(condition.After.CardID)))
	}
	if len(condition.Types) > 0 {
		types := make([]int64, len(condition.Types))
		for i, cardType := range condition.Types {
			types[i] = int64(cardType)
		}
		where = append(where, fmt.Sprintf("card_type = ANY(%s)", arg(pq.Array(types))))
	}
	if condition.NameContains != "" {
		where = append(where, fmt.Sprintf(`sort_name LIKE '%%' || %s || '%%' ESCAPE '\'`, arg(likeEscaper.Replace(condition.NameContains))))
	}
	sql := "SELECT card_id, card_type, sort_name, metadata FROM card_lists"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY card_type, sort_name, card_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.CardListRow
	for rows.Next() {
		var (
			cardID   uuid.UUID
			cardType int16
			sortName string
			raw      []byte
		)
		if err := rows.Scan(&cardID, &cardType, &sortName, &raw); err != nil {
			return nil, err
		}
		var metadata readmodel.CardListMetadata
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("card_lists.metadata of %s: %w", cardID, err)
		}
		typ, err := readmodel.NewCardType(cardType)
		if err != nil {
			return nil, err
		}
		card := query.Card{ID: metadata.ID, Type: typ, Name: metadata.Name, RegisteredAt: metadata.RegisteredAt, UpdatedAt: metadata.UpdatedAt}
		cursor := query.CardListCursor{CardType: typ, SortName: sortName, CardID: cardID}
		result = append(result, query.CardListRow{Card: card, Cursor: cursor})
	}
	return result, rows.Err()
}

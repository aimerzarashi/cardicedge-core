package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

type CollectionReader struct{}

func NewCollectionReader() *CollectionReader {
	return &CollectionReader{}
}

// 所持品の metadata.cards を行に開き、写した cards と公開 ID でつないで、種類 → 正規化した名前 → 公開 ID の順に Limit 件まで読む
// （並びは cards の索引 (card_type, sort_name, card_id) と同じ。続きは行の比べ方で取る）。sqlboiler を使わないのは、行の比べ方と
// JSON を行に開く関数が条件の部品に無く、SQL の断片を書くだけになるため（CardReader.List と同じ）。
// 写していないカード（cards に行が無い）は、種類の絞り込みや並びの中で黙って落ちたり末尾に回ったりしないよう、先に調べてエラーにする
func (r *CollectionReader) List(ctx context.Context, playerID uuid.UUID, condition query.ListCollectionCondition) ([]query.CollectionListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	const owned = `collections o CROSS JOIN LATERAL jsonb_to_recordset(o.metadata -> 'cards') AS x(card_id uuid, count int)`
	var missing []uuid.UUID
	missingRows, err := exec.QueryContext(ctx,
		`SELECT x.card_id FROM `+owned+` LEFT JOIN cards c ON c.card_id = x.card_id WHERE o.player_id = $1 AND c.card_id IS NULL`, playerID)
	if err != nil {
		return nil, err
	}
	defer missingRows.Close()
	for missingRows.Next() {
		var id uuid.UUID
		if err := missingRows.Scan(&id); err != nil {
			return nil, err
		}
		missing = append(missing, id)
	}
	if err := missingRows.Err(); err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("card %s is not published to game", missing[0])
	}

	where := []string{"o.player_id = $1"}
	args := []any{playerID}
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if len(condition.Types) > 0 {
		types := make([]int64, len(condition.Types))
		for i, cardType := range condition.Types {
			types[i] = int64(cardType)
		}
		where = append(where, fmt.Sprintf("c.card_type = ANY(%s)", arg(pq.Array(types))))
	}
	if condition.After != nil {
		where = append(where, fmt.Sprintf("(c.card_type, c.sort_name, c.card_id) > (%s, %s, %s)",
			arg(int16(condition.After.CardType)), arg(condition.After.SortName), arg(condition.After.CardID)))
	}
	sql := `SELECT c.card_id, c.card_type, c.sort_name, c.metadata, x.count FROM ` + owned +
		` JOIN cards c ON c.card_id = x.card_id WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY c.card_type, c.sort_name, c.card_id LIMIT ` + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.CollectionListRow
	for rows.Next() {
		var (
			id       uuid.UUID
			cardType int16
			sortName string
			raw      []byte
			count    int
		)
		if err := rows.Scan(&id, &cardType, &sortName, &raw, &count); err != nil {
			return nil, err
		}
		loaded, err := newCardRow(id, cardType, sortName, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.CollectionListCursor{CardType: loaded.card.Type, SortName: sortName, CardID: id}
		owned := query.CollectionCard{Card: toCard(loaded.card), Count: count}
		result = append(result, query.CollectionListRow{Card: owned, Cursor: cursor})
	}
	return result, rows.Err()
}

func toCard(card query.DeckCard) query.Card {
	return query.Card{ID: card.CardID, Name: card.Name, Type: card.Type, Cost: card.Cost, Rarity: card.Rarity, Creature: card.Creature}
}

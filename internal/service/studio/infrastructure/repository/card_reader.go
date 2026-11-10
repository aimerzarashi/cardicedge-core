package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// LIKE の記号（% _）とエスケープの文字を、文字どおりに比べるため
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// 並び順は索引（card_type, sort_name, card_id）と同じにし、続きは行の比べ方（(a, b, c) > (x, y, z)）で取る。
// sqlboiler を使わないのは、行の比べ方と LIKE の ESCAPE が sqlboiler の条件の部品に無く、qm.Where に SQL の断片を書くだけになるため
// （SQL を 1 つに書くほうが、索引と並び順の対応を読み取りやすい）。
// sort_name は列に COLLATE "C" を付けているため、並べ替えと比べ方が OS の照合順序に左右されず一致する
// cards を読む（一覧と、ほかの read model から引くカード）
type CardReader struct{}

func NewCardReader() *CardReader {
	return &CardReader{}
}

func (r *CardReader) List(ctx context.Context, condition query.ListCardsCondition) ([]query.CardListRow, error) {
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
	sql := "SELECT card_id, card_type, sort_name, metadata FROM cards"
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
		var metadata readmodel.CardMetadata
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("cards.metadata of %s: %w", cardID, err)
		}
		typ, err := readmodel.NewCardType(cardType)
		if err != nil {
			return nil, err
		}
		card := toCard(typ, metadata)
		cursor := query.CardListCursor{CardType: typ, SortName: sortName, CardID: cardID}
		result = append(result, query.CardListRow{Card: card, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *CardReader) Get(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]query.Card, error) {
	cards := make(map[uuid.UUID]query.Card, len(ids))
	if len(ids) == 0 {
		return cards, nil
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, id.String())
	}
	rows, err := models.Cards(models.CardWhere.CardID.IN(keys)).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		var metadata readmodel.CardMetadata
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("cards.metadata of %s: %w", row.CardID, err)
		}
		typ, err := readmodel.NewCardType(row.CardType)
		if err != nil {
			return nil, err
		}
		cards[metadata.ID] = toCard(typ, metadata)
	}
	return cards, nil
}

func toCard(cardType readmodel.CardType, metadata readmodel.CardMetadata) query.Card {
	return query.Card{
		ID:           metadata.ID,
		Type:         cardType,
		Name:         metadata.Name,
		Status:       metadata.Status,
		Cost:         metadata.Cost,
		Rarity:       metadata.Rarity,
		Creature:     metadata.Creature,
		Item:         metadata.Item,
		Spell:        metadata.Spell,
		Revision:     metadata.Revision,
		RegisteredAt: metadata.RegisteredAt,
		UpdatedAt:    metadata.UpdatedAt,
		RegisteredBy: metadata.RegisteredBy,
		UpdatedBy:    metadata.UpdatedBy,
	}
}

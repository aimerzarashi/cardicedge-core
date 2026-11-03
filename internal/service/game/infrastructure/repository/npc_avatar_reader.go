package repository

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// core_game の npc_avatars・npc_decks（make publish-master が studio の read model から写した表。アバターは公開中・使用不可、
// デッキは写したアバターに割り当てられたものだけ）のうち、公開中だけを読む。NPC のデッキは単独では使わず、アバターを通してだけ使う
// （core#63）ので、デッキだけを問い合わせる入り口は持たない
type NPCAvatarReader struct{}

func NewNPCAvatarReader() *NPCAvatarReader {
	return &NPCAvatarReader{}
}

const npcStatusPublished = "published"

// 写した npc_avatars.metadata のうち、読む項目。studio の read model の型を import せず、この JSON の形だけを契約にする
type npcAvatarMetadata struct {
	Name string `json:"name"`
	// 無い行は、空にせず読み出しの誤りにするため、ポインターで受ける
	Decks *[]uuid.UUID `json:"decks"`
}

// 写した npc_decks.metadata のうち、読む項目
type npcDeckMetadata struct {
	Name  string `json:"name"`
	Cards []struct {
		CardID uuid.UUID `json:"card_id"`
		Count  int       `json:"count"`
	} `json:"cards"`
}

// 写した cards.metadata のうち、読む項目
type cardMetadata struct {
	Name string `json:"name"`
}

// 並びは索引（sort_name, npc_avatar_id）と同じ。続きは行の比べ方で取る（BoardReader と同じ理由で SQL を直接書く）
func (r *NPCAvatarReader) ListPublished(ctx context.Context, condition query.ListPublishedNPCAvatarsCondition) ([]query.NPCAvatarListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	sql := "SELECT npc_avatar_id, sort_name, metadata FROM npc_avatars WHERE status = $1"
	args := []any{npcStatusPublished}
	if condition.After != nil {
		sql += " AND (sort_name, npc_avatar_id) > ($2, $3)"
		args = append(args, condition.After.SortName, condition.After.NPCAvatarID)
	}
	args = append(args, condition.Limit)
	sql += fmt.Sprintf(" ORDER BY sort_name, npc_avatar_id LIMIT $%d", len(args))

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
		avatar, err := toNPCAvatarRecord(id, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.NPCAvatarListCursor{SortName: sortName, NPCAvatarID: id}
		result = append(result, query.NPCAvatarListRow{Avatar: avatar, Cursor: cursor})
	}
	return result, rows.Err()
}

// 公開中でなければ（使用不可・準備中・行が無い）query.ErrNPCAvatarNotFound
func (r *NPCAvatarReader) GetPublished(ctx context.Context, id uuid.UUID) (query.NPCAvatarRecord, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.NPCAvatarRecord{}, err
	}
	row, err := models.NPCAvatars(
		models.NPCAvatarWhere.NPCAvatarID.EQ(id.String()),
		models.NPCAvatarWhere.Status.EQ(npcStatusPublished),
		qm.Limit(1),
	).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.NPCAvatarRecord{}, query.ErrNPCAvatarNotFound
	}
	if err != nil {
		return query.NPCAvatarRecord{}, err
	}
	return toNPCAvatarRecord(id, row.Metadata)
}

func toNPCAvatarRecord(id uuid.UUID, raw []byte) (query.NPCAvatarRecord, error) {
	var metadata npcAvatarMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.NPCAvatarRecord{}, fmt.Errorf("npc_avatars.metadata of %s: %w", id, err)
	}
	if metadata.Decks == nil {
		return query.NPCAvatarRecord{}, fmt.Errorf("npc_avatars.metadata of %s: decks is missing", id)
	}
	return query.NPCAvatarRecord{ID: id, Name: metadata.Name, DeckIDs: *metadata.Decks}, nil
}

// 渡した ID のうち公開中のデッキを、カードの名前・種類つきで返す。デッキとカードを 1 回ずつ読む（アバターの数によらない）。
// 入れたカードは種類（Creature → Item → Spell）→ 正規化した名前 → 公開 ID の順に並べる（studio の query.CompareCards と同じ。
// 並べるのに名前と種類が要るが、写した npc_decks は持たないため、写した cards から引く）
func (r *NPCAvatarReader) ListPublishedDecks(ctx context.Context, ids []uuid.UUID) ([]query.NPCDeck, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	deckRows, err := models.NPCDecks(
		qm.WhereIn(models.NPCDeckColumns.NPCDeckID+" IN ?", uuidArgs(ids)...),
		models.NPCDeckWhere.Status.EQ(npcStatusPublished),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	metadatas := make([]npcDeckMetadata, len(deckRows))
	cardIDs := []uuid.UUID{}
	for i, row := range deckRows {
		if err := json.Unmarshal(row.Metadata, &metadatas[i]); err != nil {
			return nil, fmt.Errorf("npc_decks.metadata of %s: %w", row.NPCDeckID, err)
		}
		for _, card := range metadatas[i].Cards {
			cardIDs = append(cardIDs, card.CardID)
		}
	}
	cards, err := loadCards(ctx, exec, cardIDs)
	if err != nil {
		return nil, err
	}
	decks := make([]query.NPCDeck, 0, len(deckRows))
	for i, row := range deckRows {
		deckID, err := uuid.Parse(row.NPCDeckID)
		if err != nil {
			return nil, fmt.Errorf("npc_decks.npc_deck_id %q: %w", row.NPCDeckID, err)
		}
		counts := make([]cardCount, len(metadatas[i].Cards))
		for j, c := range metadatas[i].Cards {
			counts[j] = cardCount{CardID: c.CardID, Count: c.Count}
		}
		result, err := sortedDeckCards(counts, cards)
		if err != nil {
			return nil, fmt.Errorf("npc_decks.metadata of %s: %w", row.NPCDeckID, err)
		}
		decks = append(decks, query.NPCDeck{ID: deckID, Name: metadatas[i].Name, Cards: result})
	}
	return decks, nil
}

// デッキに入れたカードの公開 ID と枚数
type cardCount struct {
	CardID uuid.UUID
	Count  int
}

// 入れたカードを、写した cards の名前・種類つきで、種類 → 正規化した名前 → 公開 ID の順に並べる。
// 写していないカードはエラー（NPC のデッキもプレイヤーのデッキも、同じ cards から引く）
func sortedDeckCards(counts []cardCount, cards map[uuid.UUID]cardRow) ([]query.DeckCard, error) {
	deckCards := make([]cardRow, 0, len(counts))
	for _, c := range counts {
		card, ok := cards[c.CardID]
		if !ok {
			return nil, fmt.Errorf("card %s is not published to game", c.CardID)
		}
		card.card.Count = c.Count
		deckCards = append(deckCards, card)
	}
	slices.SortFunc(deckCards, func(a, b cardRow) int {
		return cmp.Or(cmp.Compare(a.card.Type, b.card.Type), strings.Compare(a.sortName, b.sortName), strings.Compare(a.card.CardID.String(), b.card.CardID.String()))
	})
	result := make([]query.DeckCard, 0, len(deckCards))
	for _, c := range deckCards {
		result = append(result, c.card)
	}
	return result, nil
}

// 並べ替えに使う sort_name を、カードと一緒に持つ
type cardRow struct {
	card     query.DeckCard
	sortName string
}

// 渡した公開 ID のカードを、写した cards から 1 回で読む
func loadCards(ctx context.Context, exec rdb.Executor, ids []uuid.UUID) (map[uuid.UUID]cardRow, error) {
	cards := map[uuid.UUID]cardRow{}
	if len(ids) == 0 {
		return cards, nil
	}
	rows, err := models.Cards(qm.WhereIn(models.CardColumns.CardID+" IN ?", uuidArgs(ids)...)).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		id, err := uuid.Parse(row.CardID)
		if err != nil {
			return nil, fmt.Errorf("cards.card_id %q: %w", row.CardID, err)
		}
		cardType, err := query.NewCardType(row.CardType)
		if err != nil {
			return nil, fmt.Errorf("cards.card_type of %s: %w", id, err)
		}
		var metadata cardMetadata
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("cards.metadata of %s: %w", id, err)
		}
		card := query.DeckCard{CardID: id, Name: metadata.Name, Type: cardType}
		cards[id] = cardRow{card: card, sortName: row.SortName}
	}
	return cards, nil
}

func uuidArgs(ids []uuid.UUID) []any {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id.String())
	}
	return args
}

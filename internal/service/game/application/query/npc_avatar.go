package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// 公開 ID に当たる、プレイヤーが解放した公開中の NPC のアバターが無い（無い・解放していない・準備中・使用不可・まだ写されていない）
var ErrNPCAvatarNotFound = errors.New("query: npc avatar not found")

// カードの種類。写した cards.card_type の値（1: Creature、2: Item、3: Spell）の並びが、そのまま表示の並びになる。
// studio の read model の型を使わず、game が読む値だけを持つ
type CardType int16

const (
	CardTypeCreature CardType = 1
	CardTypeItem     CardType = 2
	CardTypeSpell    CardType = 3
)

func NewCardType(value int16) (CardType, error) {
	cardType := CardType(value)
	if cardType < CardTypeCreature || cardType > CardTypeSpell {
		return 0, fmt.Errorf("query: unknown card type: %d", value)
	}
	return cardType, nil
}

// NPC のデッキに入っているカード 1 種類。カードは NPC のデッキを見せるのに要る名前と種類だけを持つ
// （カードの一覧・詳細を game に出す問い合わせは作らない）
type DeckCard struct {
	CardID uuid.UUID
	Name   string
	Type   CardType
	Cost   int
	// 希少度（写した cards.metadata の値。normal・strange・rare・extra）
	Rarity string
	// クリーチャーだけの性能（クリーチャー以外は nil）
	Creature *CreatureSpec
	Count    int
}

// 公開中の NPC のデッキ。Cards は種類（Creature → Item → Spell）→ 名前の順（studio のデッキのカードと同じ並び）
type NPCDeck struct {
	ID    uuid.UUID
	Name  string
	Cards []DeckCard
}

func (d NPCDeck) TotalCount() int {
	total := 0
	for _, card := range d.Cards {
		total += card.Count
	}
	return total
}

// プレイヤーが解放した公開中の NPC のアバターと、使うデッキ。Decks は割り当てた順で、公開中のものだけ（準備中・使用不可のデッキは出さない）。
// NPC のデッキは単独では使わず、アバターを通してだけ使う（core#63）
type NPCAvatar struct {
	ID    uuid.UUID
	Name  string
	Decks []NPCDeck
}

// 読み出した NPC のアバター 1 件。DeckIDs は割り当てた順（写した npc_avatars.metadata の decks）で、状態は見ていない
type NPCAvatarRecord struct {
	ID      uuid.UUID
	Name    string
	DeckIDs []uuid.UUID
}

// プレイヤーが解放した NPC のアバターのうち、公開中のものを、解放した順に返す（公開中でないものは飛ばす。解放がまだ無ければ空）
type ListUnlockedNPCAvatarsFunc func(ctx context.Context, playerID uuid.UUID) ([]NPCAvatarRecord, error)

// 渡した公開 ID のうち公開中の NPC のデッキだけを、順不同で返す（無い・公開中でないものは含めない）。
// 1 ページ分のアバターのデッキを 1 回の読み出しにまとめるため、ID の一覧で受ける
type ListPublishedNPCDecksFunc func(ctx context.Context, ids []uuid.UUID) ([]NPCDeck, error)

type NPCAvatarHandler struct {
	listUnlocked ListUnlockedNPCAvatarsFunc
	listDecks    ListPublishedNPCDecksFunc
}

func NewNPCAvatarHandler(listUnlocked ListUnlockedNPCAvatarsFunc, listDecks ListPublishedNPCDecksFunc) *NPCAvatarHandler {
	return &NPCAvatarHandler{listUnlocked: listUnlocked, listDecks: listDecks}
}

// プレイヤーが解放した NPC のアバターを、解放した順に返す（Player.npcAvatars）。解放した数は少ないので、ページ分けしない。
// デッキは全アバター分を 1 回で読む。投影が遅れて解放がまだ無いときも、空の一覧を返す（nil にしない。GraphQL の [NpcAvatar!]! は null を返せないため）
func (q *NPCAvatarHandler) ListUnlockedByPlayer(ctx context.Context, playerID uuid.UUID) ([]NPCAvatar, error) {
	records, err := q.listUnlocked(ctx, playerID)
	if err != nil {
		return nil, err
	}
	return q.withDecks(ctx, records)
}

// 解放していない・公開中でないアバターは ErrNPCAvatarNotFound（存在を明かさないため区別しない）
func (q *NPCAvatarHandler) GetUnlocked(ctx context.Context, playerID uuid.UUID, id uuid.UUID) (NPCAvatar, error) {
	avatars, err := q.ListUnlockedByPlayer(ctx, playerID)
	if err != nil {
		return NPCAvatar{}, err
	}
	for _, avatar := range avatars {
		if avatar.ID == id {
			return avatar, nil
		}
	}
	return NPCAvatar{}, ErrNPCAvatarNotFound
}

// 全アバターのデッキを 1 回で読み、アバターごとに割り当てた順に並べ直す。公開中でないデッキは読み出しの結果に無いので飛ばす。
// 同じデッキを複数のアバターが使ってよい（複製せず、同じ値を渡す）
func (q *NPCAvatarHandler) withDecks(ctx context.Context, records []NPCAvatarRecord) ([]NPCAvatar, error) {
	seen := map[uuid.UUID]struct{}{}
	ids := []uuid.UUID{}
	for _, record := range records {
		for _, id := range record.DeckIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	published := map[uuid.UUID]NPCDeck{}
	if len(ids) > 0 {
		decks, err := q.listDecks(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, deck := range decks {
			published[deck.ID] = deck
		}
	}
	avatars := make([]NPCAvatar, 0, len(records))
	for _, record := range records {
		decks := make([]NPCDeck, 0, len(record.DeckIDs))
		for _, id := range record.DeckIDs {
			if deck, ok := published[id]; ok {
				decks = append(decks, deck)
			}
		}
		avatars = append(avatars, NPCAvatar{ID: record.ID, Name: record.Name, Decks: decks})
	}
	return avatars, nil
}

package repository

import (
	"cmp"
	"context"
	"encoding/json"
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
// 項目と形は studio の readmodel.CardMetadata（make publish-master がそのまま写す）。性能が無い行（性能を足す前に投影した行）は
// 0 や空にせず読み出しの誤りにする（ボードの tile_counts と同じ。投影を作り直して写し直せば入る）
type cardMetadata struct {
	Name   string `json:"name"`
	Cost   *int   `json:"cost"`
	Rarity string `json:"rarity"`
	// クリーチャーだけが持つ入れ子（アイテム・スペルは無い）
	Creature *creatureMetadata `json:"creature"`
	// アイテムだけが持つ入れ子
	Item *itemMetadata `json:"item"`
	// スペルだけが持つ入れ子
	Spell *spellMetadata `json:"spell"`
}

type spellMetadata struct {
	// 効果なしは {}（null ではない）。無いのは効果を足す前に投影した行
	Effect *spellEffectMetadata `json:"effect"`
}

type spellEffectMetadata struct {
	DrawCards *struct {
		Count int `json:"count"`
	} `json:"draw_cards"`
	GainMagic *struct {
		PerLap int `json:"per_lap"`
	} `json:"gain_magic"`
	RollDice *struct {
		Count int `json:"count"`
	} `json:"roll_dice"`
	FixedDiceCurse *struct {
		Value int `json:"value"`
		Turns int `json:"turns"`
	} `json:"fixed_dice_curse"`
	Damage *struct {
		Amount int `json:"amount"`
	} `json:"damage"`
}

type itemMetadata struct {
	Category          string `json:"category"`
	AttackModifier    *int   `json:"attack_modifier"`
	HitPointsModifier *int   `json:"hit_points_modifier"`
}

type creatureMetadata struct {
	Elements              []string                  `json:"elements"`
	Attack                *int                      `json:"attack"`
	HitPoints             *int                      `json:"hit_points"`
	ItemRestrictions      []string                  `json:"item_restrictions"`
	PlacementRestrictions []string                  `json:"placement_restrictions"`
	Abilities             []creatureAbilityMetadata `json:"abilities"`
}

// プレイヤーが解放した NPC のアバターを、解放した順（read model unlocks の metadata の並び）に読む。公開中でないもの
// （使用不可・準備中・写していない）は飛ばす。デッキは別の読み出し（ListPublishedDecks）で、全アバター分を 1 回で引く
func (r *NPCAvatarReader) ListUnlockedByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.NPCAvatarRecord, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	unlocked, found, err := unlockMetadataOf(ctx, exec, playerID)
	if err != nil || !found || len(unlocked.NPCAvatars) == 0 {
		return nil, err
	}
	rows, err := models.NPCAvatars(
		qm.WhereIn(models.NPCAvatarColumns.NPCAvatarID+" IN ?", uuidArgs(unlocked.NPCAvatars)...),
		models.NPCAvatarWhere.Status.EQ(npcStatusPublished),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	published := make(map[uuid.UUID]*models.NPCAvatar, len(rows))
	for _, row := range rows {
		id, err := uuid.Parse(row.NPCAvatarID)
		if err != nil {
			return nil, fmt.Errorf("npc_avatars.npc_avatar_id %q: %w", row.NPCAvatarID, err)
		}
		published[id] = row
	}
	records := make([]query.NPCAvatarRecord, 0, len(unlocked.NPCAvatars))
	for _, id := range unlocked.NPCAvatars {
		row, ok := published[id]
		if !ok {
			continue
		}
		record, err := toNPCAvatarRecord(id, row.Metadata)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
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
		loaded, err := newCardRow(id, row.CardType, row.SortName, row.Metadata)
		if err != nil {
			return nil, err
		}
		cards[id] = loaded
	}
	return cards, nil
}

// 写した cards の 1 行から、見せる項目と並べ替えの値を作る。性能（コスト・希少度、クリーチャーは属性・攻撃力・体力）が無い行は、0 や空にせず読み出しの誤りにする
func newCardRow(id uuid.UUID, cardTypeValue int16, sortName string, raw []byte) (cardRow, error) {
	cardType, err := query.NewCardType(cardTypeValue)
	if err != nil {
		return cardRow{}, fmt.Errorf("cards.card_type of %s: %w", id, err)
	}
	var metadata cardMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return cardRow{}, fmt.Errorf("cards.metadata of %s: %w", id, err)
	}
	if metadata.Cost == nil || metadata.Rarity == "" {
		return cardRow{}, fmt.Errorf("cards.metadata of %s has no cost or rarity (project the cards again and publish the master)", id)
	}
	card := query.DeckCard{CardID: id, Name: metadata.Name, Type: cardType, Cost: *metadata.Cost, Rarity: metadata.Rarity}
	if cardType == query.CardTypeCreature {
		creatureSpec := metadata.Creature
		if creatureSpec == nil || len(creatureSpec.Elements) == 0 || creatureSpec.Attack == nil || creatureSpec.HitPoints == nil || creatureSpec.ItemRestrictions == nil || creatureSpec.PlacementRestrictions == nil || creatureSpec.Abilities == nil {
			return cardRow{}, fmt.Errorf("cards.metadata of %s has no creature elements, attack, hit_points, item_restrictions, placement_restrictions or abilities (project the cards again and publish the master)", id)
		}
		card.Creature = &query.CreatureSpec{Elements: creatureSpec.Elements, Attack: *creatureSpec.Attack, HitPoints: *creatureSpec.HitPoints, ItemRestrictions: creatureSpec.ItemRestrictions, PlacementRestrictions: creatureSpec.PlacementRestrictions, Abilities: queryCreatureAbilities(creatureSpec.Abilities)}
	}
	if cardType == query.CardTypeItem {
		itemSpec := metadata.Item
		if itemSpec == nil || itemSpec.Category == "" || itemSpec.AttackModifier == nil || itemSpec.HitPointsModifier == nil {
			return cardRow{}, fmt.Errorf("cards.metadata of %s has no item category, attack_modifier or hit_points_modifier (project the cards again and publish the master)", id)
		}
		card.Item = &query.ItemSpec{Category: itemSpec.Category, AttackModifier: *itemSpec.AttackModifier, HitPointsModifier: *itemSpec.HitPointsModifier}
	}
	if cardType == query.CardTypeSpell {
		spellSpec := metadata.Spell
		if spellSpec == nil || spellSpec.Effect == nil {
			return cardRow{}, fmt.Errorf("cards.metadata of %s has no spell effect (project the cards again and publish the master)", id)
		}
		var effect query.SpellEffect
		if spellSpec.Effect.DrawCards != nil {
			effect.DrawCards = &query.SpellDrawCards{Count: spellSpec.Effect.DrawCards.Count}
		}
		if spellSpec.Effect.GainMagic != nil {
			effect.GainMagic = &query.SpellGainMagic{PerLap: spellSpec.Effect.GainMagic.PerLap}
		}
		if spellSpec.Effect.RollDice != nil {
			effect.RollDice = &query.SpellRollDice{Count: spellSpec.Effect.RollDice.Count}
		}
		if curse := spellSpec.Effect.FixedDiceCurse; curse != nil {
			effect.FixedDiceCurse = &query.SpellFixedDiceCurse{Value: curse.Value, Turns: curse.Turns}
		}
		if damage := spellSpec.Effect.Damage; damage != nil {
			effect.Damage = &query.SpellDamage{Amount: damage.Amount}
		}
		card.Spell = &query.SpellSpec{Effect: effect}
	}
	return cardRow{card: card, sortName: sortName}, nil
}

func uuidArgs(ids []uuid.UUID) []any {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id.String())
	}
	return args
}

type creatureAbilityMetadata struct {
	Defensive      *struct{} `json:"defensive"`
	TollMultiplier *struct {
		Percent int `json:"percent"`
	} `json:"toll_multiplier"`
	FirstStrike    *struct{} `json:"first_strike"`
	LastStrike     *struct{} `json:"last_strike"`
	BattleModifier *struct {
		AttackModifier    int `json:"attack_modifier"`
		HitPointsModifier int `json:"hit_points_modifier"`
	} `json:"battle_modifier"`
}

func queryCreatureAbilities(abilities []creatureAbilityMetadata) []query.CreatureAbility {
	values := make([]query.CreatureAbility, len(abilities))
	for i, ability := range abilities {
		values[i] = query.CreatureAbility{Defensive: ability.Defensive != nil, FirstStrike: ability.FirstStrike != nil, LastStrike: ability.LastStrike != nil}
		if ability.TollMultiplier != nil {
			values[i].TollMultiplierPercent = &ability.TollMultiplier.Percent
		}
		if ability.BattleModifier != nil {
			values[i].BattleModifier = &query.CreatureBattleModifier{AttackModifier: ability.BattleModifier.AttackModifier, HitPointsModifier: ability.BattleModifier.HitPointsModifier}
		}
	}
	return values
}

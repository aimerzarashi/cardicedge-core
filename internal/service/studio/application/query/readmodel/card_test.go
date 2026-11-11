package readmodel_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item/itemtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestCard_EvolvesFromCreatureRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredCreature(t, publicID, "Ｄragon", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｄragon", Status: readmodel.CardStatusDraft, Cost: 10, Rarity: "normal", Creature: &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, Revision: 1, RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeCreature,
		Status:   readmodel.CardStatusDraft,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName: "dragon",
		Metadata: metadata,
		// 登録の直後は、更新した人も登録した人
		RegisteredBy: operatortest.ID(t, 10), UpdatedBy: operatortest.ID(t, 10),
	}, state)
	require.False(t, state.IsTerminal())
}

func registeredItem(t *testing.T, publicID uuid.UUID, name string, at time.Time) []item.Event {
	t.Helper()
	events, err := item.Decide(item.InitialState(), &item.RegisterCommand{
		ID: itemtest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}

func TestCard_EvolvesFromItemRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredItem(t, publicID, "Ｐotion", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｐotion", Status: readmodel.CardStatusDraft, Cost: 10, Rarity: "normal", Item: &readmodel.ItemMetadata{Category: "weapon"}, Revision: 1, RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:       publicID,
		CardType:     readmodel.CardTypeItem,
		Status:       readmodel.CardStatusDraft,
		SortName:     "potion",
		Metadata:     metadata,
		RegisteredBy: operatortest.ID(t, 10), UpdatedBy: operatortest.ID(t, 10),
	}, state)
	require.False(t, state.IsTerminal())
}

func registeredSpell(t *testing.T, publicID uuid.UUID, name string, at time.Time) []spell.Event {
	t.Helper()
	events, err := spell.Decide(spell.InitialState(), &spell.RegisterCommand{
		ID: spelltest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}

func TestCard_EvolvesFromSpellRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredSpell(t, publicID, "Ｆireball", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{
		ID: publicID, Name: "Ｆireball", Status: readmodel.CardStatusDraft, Cost: 10, Rarity: "normal", Revision: 1, RegisteredAt: at, UpdatedAt: at,
		Spell: &readmodel.SpellMetadata{},
	}
	require.Equal(t, readmodel.Card{
		CardID:       publicID,
		CardType:     readmodel.CardTypeSpell,
		Status:       readmodel.CardStatusDraft,
		SortName:     "fireball",
		Metadata:     metadata,
		RegisteredBy: operatortest.ID(t, 10), UpdatedBy: operatortest.ID(t, 10),
	}, state)
	require.False(t, state.IsTerminal())
}

func TestCard_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.Card{CardID: uuid.New(), CardType: readmodel.CardTypeCreature}

	evolved := state.Evolve(&operator.RegisteredEvent{})

	require.Equal(t, state, evolved)
}

// 公開・使用不可のイベントは、状態の列と metadata の状態・revision・最後に変更した日時と人を変える（登録した日時と人は変えない）
func TestCard_EvolvesStatusFromPublishedAndRetired(t *testing.T) {
	registeredAt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	publishedAt := registeredAt.Add(time.Hour)
	retiredAt := publishedAt.Add(time.Hour)
	publicID := uuid.New()
	state := readmodel.Card{}.Evolve(registeredCreature(t, publicID, "Dragon", registeredAt)[0])
	registered := creature.Creature{ID: creaturetest.ID(t, 1), Version: 1, Status: card.StatusDraft}
	published, err := creature.Decide(registered, &creature.PublishCommand{PublishedBy: operatortest.ID(t, 11)}, publishedAt)
	require.NoError(t, err)
	publishedState, err := creature.Evolve(registered, published[0])
	require.NoError(t, err)
	retired, err := creature.Decide(publishedState, &creature.RetireCommand{RetiredBy: operatortest.ID(t, 12)}, retiredAt)
	require.NoError(t, err)

	afterPublished := state.Evolve(published[0])
	afterRetired := afterPublished.Evolve(retired[0])

	require.Equal(t, readmodel.CardStatusPublished, afterPublished.Status)
	require.Equal(t, readmodel.CardMetadata{
		ID: publicID, Name: "Dragon", Status: readmodel.CardStatusPublished, Cost: 10, Rarity: "normal", Creature: &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, Revision: 2, RegisteredAt: registeredAt, UpdatedAt: publishedAt,
	}, afterPublished.Metadata)
	require.Equal(t, readmodel.CardStatusRetired, afterRetired.Status)
	require.Equal(t, readmodel.CardMetadata{
		ID: publicID, Name: "Dragon", Status: readmodel.CardStatusRetired, Cost: 10, Rarity: "normal", Creature: &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, Revision: 3, RegisteredAt: registeredAt, UpdatedAt: retiredAt,
	}, afterRetired.Metadata)
	require.Equal(t, operatortest.ID(t, 10), afterPublished.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 11), afterPublished.UpdatedBy)
	require.Equal(t, operatortest.ID(t, 10), afterRetired.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 12), afterRetired.UpdatedBy)
	require.False(t, afterRetired.IsTerminal())
}

func TestNewCardStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewCardStatus("archived")

	require.Error(t, err)
}

func TestCard_EvolvesSpecFromCostAndRarityChanged(t *testing.T) {
	registeredAt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	changedAt := registeredAt.Add(time.Hour)
	publicID := uuid.New()
	state := readmodel.Card{}.Evolve(registeredCreature(t, publicID, "Dragon", registeredAt)[0])
	registered := creature.Creature{ID: creaturetest.ID(t, 1), Version: 1, Status: card.StatusDraft, Cost: card.DefaultCost, Rarity: card.DefaultRarity,
		Elements: creature.DefaultElements(), Attack: creature.DefaultAttack, HitPoints: creature.DefaultHitPoints, ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities()}
	costChanged, err := creature.Decide(registered, &creature.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: operatortest.ID(t, 11)}, changedAt)
	require.NoError(t, err)
	afterCostState, err := creature.Evolve(registered, costChanged[0])
	require.NoError(t, err)
	rarityChanged, err := creature.Decide(afterCostState, &creature.ChangeRarityCommand{Rarity: card.RarityRare, ChangedBy: operatortest.ID(t, 12)}, changedAt)
	require.NoError(t, err)

	after := state.Evolve(costChanged[0]).Evolve(rarityChanged[0])

	require.Equal(t, readmodel.CardMetadata{
		ID: publicID, Name: "Dragon", Status: readmodel.CardStatusDraft, Cost: 30, Rarity: "rare", Creature: &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, Revision: 3, RegisteredAt: registeredAt, UpdatedAt: changedAt,
	}, after.Metadata)
	require.Equal(t, operatortest.ID(t, 12), after.UpdatedBy)
}

func TestCard_EvolvesCreatureSpecFromElementsAndStatsChanged(t *testing.T) {
	registeredAt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	changedAt := registeredAt.Add(time.Hour)
	publicID := uuid.New()
	registeredState := readmodel.Card{}.Evolve(registeredCreature(t, publicID, "Dragon", registeredAt)[0])
	registered := creature.Creature{
		ID: creaturetest.ID(t, 1), Version: 1, Status: card.StatusDraft, Cost: card.DefaultCost, Rarity: card.DefaultRarity,
		Elements: creature.DefaultElements(), Attack: creature.DefaultAttack, HitPoints: creature.DefaultHitPoints, ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities(),
	}
	elementsChanged, err := creature.Decide(registered, &creature.ChangeElementsCommand{Elements: creaturetest.Elements(t, "wind", "fire"), ChangedBy: operatortest.ID(t, 11)}, changedAt)
	require.NoError(t, err)
	afterElementsState, err := creature.Evolve(registered, elementsChanged[0])
	require.NoError(t, err)
	statsChanged, err := creature.Decide(afterElementsState, &creature.ChangeStatsCommand{Attack: creaturetest.Attack(t, 30), HitPoints: creaturetest.HitPoints(t, 50), ChangedBy: operatortest.ID(t, 12)}, changedAt)
	require.NoError(t, err)

	afterElements := registeredState.Evolve(elementsChanged[0])
	afterStats := afterElements.Evolve(statsChanged[0])

	require.Equal(t, &readmodel.CreatureMetadata{Elements: []string{"fire", "wind"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, afterElements.Metadata.Creature)
	require.Equal(t, &readmodel.CreatureMetadata{Elements: []string{"fire", "wind"}, Attack: 30, HitPoints: 50, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, afterStats.Metadata.Creature)
	require.Equal(t, int64(3), afterStats.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 12), afterStats.UpdatedBy)
	restrictionsChanged := creature.ItemRestrictionsChangedEvent{ItemRestrictions: creaturetest.ItemRestrictions(t, "armor", "scroll"), ChangedBy: operatortest.ID(t, 14)}
	afterRestrictions := afterStats.Evolve(&restrictionsChanged)
	require.Equal(t, &readmodel.CreatureMetadata{Elements: []string{"fire", "wind"}, Attack: 30, HitPoints: 50, ItemRestrictions: []string{"armor", "scroll"}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, afterRestrictions.Metadata.Creature)
	placementChanged := creature.PlacementRestrictionsChangedEvent{PlacementRestrictions: creaturetest.PlacementRestrictions(t, "fire"), ChangedBy: operatortest.ID(t, 15)}
	afterPlacement := afterRestrictions.Evolve(&placementChanged)
	require.Equal(t, &readmodel.CreatureMetadata{Elements: []string{"fire", "wind"}, Attack: 30, HitPoints: 50, ItemRestrictions: []string{"armor", "scroll"}, PlacementRestrictions: []string{"fire"}, Abilities: []readmodel.CreatureAbilityMetadata{}}, afterPlacement.Metadata.Creature)
	defensive, err := creature.NewAbilities([]creature.Ability{creature.NewDefensiveAbility()})
	require.NoError(t, err)
	abilitiesChanged := creature.AbilitiesChangedEvent{Abilities: defensive, ChangedBy: operatortest.ID(t, 16)}
	afterAbilities := afterPlacement.Evolve(&abilitiesChanged)
	require.Equal(t, []readmodel.CreatureAbilityMetadata{{Defensive: &readmodel.CreatureDefensiveAbilityMetadata{}}}, afterAbilities.Metadata.Creature.Abilities)
	percent, err := creature.NewTollMultiplierPercent(150)
	require.NoError(t, err)
	tolled, err := creature.NewAbilities([]creature.Ability{creature.NewTollMultiplierAbility(percent)})
	require.NoError(t, err)
	tollChanged := creature.AbilitiesChangedEvent{Abilities: tolled, ChangedBy: operatortest.ID(t, 17)}
	require.Equal(t, []readmodel.CreatureAbilityMetadata{{TollMultiplier: &readmodel.CreatureTollMultiplierAbilityMetadata{Percent: 150}}}, afterAbilities.Evolve(&tollChanged).Metadata.Creature.Abilities)
	firstStrike, err := creature.NewAbilities([]creature.Ability{creature.NewFirstStrikeAbility()})
	require.NoError(t, err)
	firstStrikeChanged := creature.AbilitiesChangedEvent{Abilities: firstStrike, ChangedBy: operatortest.ID(t, 18)}
	require.Equal(t, []readmodel.CreatureAbilityMetadata{{FirstStrike: &readmodel.CreatureFirstStrikeAbilityMetadata{}}}, afterAbilities.Evolve(&firstStrikeChanged).Metadata.Creature.Abilities)
	lastStrike, err := creature.NewAbilities([]creature.Ability{creature.NewLastStrikeAbility()})
	require.NoError(t, err)
	lastStrikeChanged := creature.AbilitiesChangedEvent{Abilities: lastStrike, ChangedBy: operatortest.ID(t, 19)}
	require.Equal(t, []readmodel.CreatureAbilityMetadata{{LastStrike: &readmodel.CreatureLastStrikeAbilityMetadata{}}}, afterAbilities.Evolve(&lastStrikeChanged).Metadata.Creature.Abilities)
	// 前の状態の値を書き換えない（投影が途中の状態を持ち回るため）
	require.Equal(t, &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}, registeredState.Metadata.Creature)
}

// アイテム・呪文は、クリーチャーだけの性能を持たない（JSON にも出さない）
func TestCard_ItemAndSpellHaveNoCreatureSpec(t *testing.T) {
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

	item := readmodel.Card{}.Evolve(registeredItem(t, uuid.New(), "Potion", at)[0])
	spell := readmodel.Card{}.Evolve(registeredSpell(t, uuid.New(), "Fireball", at)[0])

	require.Nil(t, item.Metadata.Creature)
	require.Nil(t, spell.Metadata.Creature)
}

func TestCard_ItemHasItsSpecFromRegistrationAndChanges(t *testing.T) {
	at := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	registered := readmodel.Card{}.Evolve(registeredItem(t, uuid.New(), "Potion", at)[0])
	require.Equal(t, &readmodel.ItemMetadata{Category: "weapon"}, registered.Metadata.Item)
	meta := item.EventMeta{EntityID: itemtest.ID(t, 1), AggregateVersion: 2, OccurredAt: at}

	categorized := registered.Evolve(&item.CategoryChangedEvent{EventMeta: meta, Category: item.CategoryArmor, ChangedBy: operatortest.ID(t, 10)})
	changed := categorized.Evolve(&item.StatBonusesChangedEvent{EventMeta: meta, AttackBonus: item.NewAttackBonus(5), HitPointsBonus: item.NewHitPointsBonus(30), ChangedBy: operatortest.ID(t, 10)})

	require.Equal(t, &readmodel.ItemMetadata{Category: "armor", AttackBonus: 5, HitPointsBonus: 30}, changed.Metadata.Item)
	// 元の状態の入れ子を書き換えない
	require.Equal(t, "weapon", registered.Metadata.Item.Category)
}

func TestCard_EvolvesSpellEffectFromEffectChanged(t *testing.T) {
	registeredAt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	changedAt := registeredAt.Add(time.Hour)
	events := registeredSpell(t, uuid.New(), "Hope", registeredAt)
	registeredState := readmodel.Card{}.Evolve(events[0])
	registered, err := spell.Evolve(spell.InitialState(), events[0])
	require.NoError(t, err)
	changed, err := spell.Decide(registered, &spell.ChangeEffectCommand{Effect: spelltest.DrawCards(t, 2), ChangedBy: operatortest.ID(t, 11)}, changedAt)
	require.NoError(t, err)

	afterEffect := registeredState.Evolve(changed[0])

	drawTwo := readmodel.SpellEffectMetadata{DrawCards: &readmodel.SpellDrawCardsMetadata{Count: 2}}
	require.Equal(t, &readmodel.SpellMetadata{Effect: drawTwo}, afterEffect.Metadata.Spell)
	require.Equal(t, int64(2), afterEffect.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 11), afterEffect.UpdatedBy)
	// 前の状態の値を書き換えない（投影が途中の状態を持ち回るため）
	require.Equal(t, &readmodel.SpellMetadata{}, registeredState.Metadata.Spell)
}

// 効果の JSON は domain の spell.Effect と同じ形（game が読む約束。効果なしは {}）
func TestSpellMetadata_EffectJSONMatchesTheDomainEffect(t *testing.T) {
	var none readmodel.SpellEffectMetadata
	drawOne := readmodel.SpellEffectMetadata{DrawCards: &readmodel.SpellDrawCardsMetadata{Count: 1}}
	tests := map[string]struct {
		effect readmodel.SpellEffectMetadata
		want   string
	}{
		"no_effect":  {effect: none, want: `{"effect":{}}`},
		"draw_cards": {effect: drawOne, want: `{"effect":{"draw_cards":{"count":1}}}`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(readmodel.SpellMetadata{Effect: tt.effect})
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(raw))
		})
	}
}

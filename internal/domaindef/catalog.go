package domaindef

// 種別の名前と id の一覧（全サービスで共通。core_studio・core_game のどちらの DB にも、同じ行を入れる）。
// id は自動で振らない（DB ごと・起動の順番で変わりうるため）。一度決めた名前と id は変えない・消さない（過去のイベントが id で
// 参照しているため）。ただし main に入る前（リリース前）は除く（core#42 で配布デッキセットの種別を、core#92 で配布デッキの
// 公開中の数の上限 grant_deck.published（1001）を、開発の DB を作り直す前提で消した。消した番号は使い回さず、スターターセットの公開中の上限には 1003 を振った）。
// 名前は各集約の宣言（Types() 等）にも書くが、ドメインのコードには id を持たせない（保存の都合のため）。
// この一覧と DB は Sync が合わせ、DB と各集約の宣言は Verify... が照合する（core#33）
//
// 振り方: 集約は 1000 から 100 刻み、イベントはその集約の id ＋ 1 から順に、一意性の key・参照・上限は 1000 から順に。
// テスト用の範囲は持たない（偽の種別を足すテストは複製した DB を使う）
var Types = Catalog{
	EntityTypes: []EntityTypeDef{
		{ID: 1000, Name: "operator", Events: []EventTypeDef{
			{ID: 1001, Name: "RegisteredEvent"},
			{ID: 1002, Name: "LinkedEvent"},
		}},
		{ID: 1100, Name: "creature", Events: []EventTypeDef{
			{ID: 1101, Name: "RegisteredEvent"},
			{ID: 1102, Name: "PublishedEvent"},
			{ID: 1103, Name: "RetiredEvent"},
			{ID: 1104, Name: "CostChangedEvent"},
			{ID: 1105, Name: "RarityChangedEvent"},
			{ID: 1106, Name: "ElementsChangedEvent"},
			{ID: 1107, Name: "StatsChangedEvent"},
			{ID: 1109, Name: "ItemRestrictionsChangedEvent"},
			{ID: 1110, Name: "PlacementRestrictionsChangedEvent"},
			{ID: 1111, Name: "EffectsChangedEvent"},
		}},
		{ID: 1200, Name: "item", Events: []EventTypeDef{
			{ID: 1201, Name: "RegisteredEvent"},
			{ID: 1202, Name: "PublishedEvent"},
			{ID: 1203, Name: "RetiredEvent"},
			{ID: 1204, Name: "CostChangedEvent"},
			{ID: 1205, Name: "RarityChangedEvent"},
			{ID: 1206, Name: "CategoryChangedEvent"},
			{ID: 1207, Name: "EffectsChangedEvent"},
		}},
		{ID: 1300, Name: "spell", Events: []EventTypeDef{
			{ID: 1301, Name: "RegisteredEvent"},
			{ID: 1302, Name: "PublishedEvent"},
			{ID: 1303, Name: "RetiredEvent"},
			{ID: 1304, Name: "CostChangedEvent"},
			{ID: 1305, Name: "RarityChangedEvent"},
			{ID: 1306, Name: "EffectChangedEvent"},
		}},
		{ID: 1400, Name: "preset_deck", Events: []EventTypeDef{
			{ID: 1401, Name: "RegisteredEvent"},
			{ID: 1402, Name: "RenamedEvent"},
			{ID: 1403, Name: "CardCountsChangedEvent"},
			{ID: 1404, Name: "DeletedEvent"},
		}},
		{ID: 1600, Name: "player", Events: []EventTypeDef{
			{ID: 1601, Name: "RegisteredEvent"},
		}},
		{ID: 1700, Name: "starter_grant", Events: []EventTypeDef{
			{ID: 1701, Name: "StartedEvent"},
			{ID: 1702, Name: "CollectionRecordedEvent"},
			{ID: 1703, Name: "DeckRecordedEvent"},
			{ID: 1704, Name: "CompletedEvent"},
			{ID: 1705, Name: "UnlockRecordedEvent"},
		}},
		{ID: 1800, Name: "collection", Events: []EventTypeDef{
			{ID: 1801, Name: "CreatedEvent"},
			{ID: 1802, Name: "CardsAcquiredEvent"},
		}},
		{ID: 1900, Name: "deck", Events: []EventTypeDef{
			{ID: 1901, Name: "CreatedEvent"},
		}},
		{ID: 2000, Name: "game_rules", Events: []EventTypeDef{
			{ID: 2001, Name: "CreatedEvent"},
		}},
		{ID: 2100, Name: "grant_deck", Events: []EventTypeDef{
			{ID: 2101, Name: "RegisteredEvent"},
			{ID: 2102, Name: "RenamedEvent"},
			{ID: 2103, Name: "CardCountsChangedEvent"},
			{ID: 2104, Name: "PublishedEvent"},
			{ID: 2105, Name: "RetiredEvent"},
			{ID: 2106, Name: "DeletedEvent"},
		}},
		{ID: 2200, Name: "avatar", Events: []EventTypeDef{
			{ID: 2201, Name: "RegisteredEvent"},
		}},
		{ID: 2300, Name: "board", Events: []EventTypeDef{
			{ID: 2301, Name: "RegisteredEvent"},
			{ID: 2302, Name: "RenamedEvent"},
			{ID: 2303, Name: "LayoutChangedEvent"},
			{ID: 2304, Name: "PublishedEvent"},
			{ID: 2305, Name: "RetiredEvent"},
			{ID: 2306, Name: "DeletedEvent"},
			{ID: 2307, Name: "DefaultRulesChangedEvent"},
		}},
		{ID: 2400, Name: "studio_rules", Events: []EventTypeDef{
			{ID: 2401, Name: "CreatedEvent"},
		}},
		{ID: 2500, Name: "npc_avatar", Events: []EventTypeDef{
			{ID: 2501, Name: "RegisteredEvent"},
			{ID: 2502, Name: "RenamedEvent"},
			{ID: 2503, Name: "DeletedEvent"},
			{ID: 2504, Name: "PublishedEvent"},
			{ID: 2505, Name: "RetiredEvent"},
			{ID: 2506, Name: "DecksAssignedEvent"},
			{ID: 2507, Name: "DecksUnassignedEvent"},
		}},
		{ID: 2600, Name: "npc_deck", Events: []EventTypeDef{
			{ID: 2601, Name: "RegisteredEvent"},
			{ID: 2602, Name: "RenamedEvent"},
			{ID: 2603, Name: "CardCountsChangedEvent"},
			{ID: 2604, Name: "PublishedEvent"},
			{ID: 2605, Name: "RetiredEvent"},
			{ID: 2606, Name: "DeletedEvent"},
		}},
		{ID: 2700, Name: "starter_set", Events: []EventTypeDef{
			{ID: 2701, Name: "RegisteredEvent"},
			{ID: 2702, Name: "RenamedEvent"},
			{ID: 2703, Name: "DeletedEvent"},
			{ID: 2704, Name: "PublishedEvent"},
			{ID: 2705, Name: "RetiredEvent"},
			{ID: 2706, Name: "GrantDecksAssignedEvent"},
			{ID: 2707, Name: "GrantDecksUnassignedEvent"},
			{ID: 2708, Name: "UnlockBoardsAssignedEvent"},
			{ID: 2709, Name: "UnlockBoardsUnassignedEvent"},
			{ID: 2710, Name: "UnlockNPCAvatarsAssignedEvent"},
			{ID: 2711, Name: "UnlockNPCAvatarsUnassignedEvent"},
		}},
		{ID: 2800, Name: "unlock", Events: []EventTypeDef{
			{ID: 2801, Name: "CreatedEvent"},
			{ID: 2802, Name: "BoardsUnlockedEvent"},
			{ID: 2803, Name: "NPCAvatarsUnlockedEvent"},
		}},
		{ID: 2900, Name: "room", Events: []EventTypeDef{
			{ID: 2901, Name: "CreatedEvent"},
			{ID: 2902, Name: "SeatsChangedEvent"},
			{ID: 2903, Name: "MatchRulesChangedEvent"},
			{ID: 2904, Name: "SeatChoiceChangedEvent"},
			{ID: 2905, Name: "PublishedEvent"},
			{ID: 2906, Name: "PreparationStartedEvent"},
			{ID: 2907, Name: "ReadyDeclaredEvent"},
			{ID: 2908, Name: "ClosedEvent"},
			{ID: 2909, Name: "RenamedEvent"},
			{ID: 2910, Name: "MatchStartedEvent"},
			{ID: 2911, Name: "MatchFinishedEvent"},
			{ID: 2912, Name: "NextDeclaredEvent"},
			{ID: 2913, Name: "PreparationRestartedEvent"},
			{ID: 2914, Name: "PausedEvent"},
		}},
		{ID: 3000, Name: "match", Events: []EventTypeDef{
			{ID: 3001, Name: "CreatedEvent"},
			{ID: 3002, Name: "TurnOrderDecidedEvent"},
			{ID: 3003, Name: "DrawPilesShuffledEvent"},
			{ID: 3004, Name: "HandsDealtEvent"},
			{ID: 3005, Name: "CardDrawnEvent"},
			{ID: 3006, Name: "CardDiscardedEvent"},
			{ID: 3007, Name: "DiceRolledEvent"},
			{ID: 3008, Name: "PieceMovedEvent"},
			{ID: 3009, Name: "CheckpointPassedEvent"},
			{ID: 3010, Name: "LapCompletedEvent"},
			{ID: 3011, Name: "TurnEndedEvent"},
			{ID: 3012, Name: "MatchEndedEvent"},
			{ID: 3013, Name: "RewardDrawnEvent"},
			{ID: 3014, Name: "AllRewardsReceivedEvent"},
			{ID: 3015, Name: "CreaturePlacedEvent"},
			{ID: 3016, Name: "TollPaidEvent"},
			{ID: 3017, Name: "TerritoryRelinquishedEvent"},
			{ID: 3018, Name: "MagicDepletedEvent"},
			{ID: 3019, Name: "TerritoryInvadedEvent"},
			{ID: 3020, Name: "ItemUsedEvent"},
			{ID: 3021, Name: "CreatureStruckEvent"},
			{ID: 3022, Name: "InvasionSucceededEvent"},
			{ID: 3023, Name: "InvasionFailedEvent"},
			{ID: 3024, Name: "ItemSkippedEvent"},
			{ID: 3025, Name: "BattleStartedEvent"},
			{ID: 3026, Name: "BattleSupportEffectActivatedEvent"},
			{ID: 3027, Name: "BattleTerrainEffectActivatedEvent"},
			{ID: 3028, Name: "BattleCreatureEffectActivatedEvent"},
			{ID: 3029, Name: "BattleItemEffectActivatedEvent"},
			{ID: 3030, Name: "LandLeveledUpEvent"},
			{ID: 3031, Name: "LandElementChangedEvent"},
			{ID: 3032, Name: "CreatureExchangedEvent"},
			{ID: 3033, Name: "CreatureMovedEvent"},
			{ID: 3034, Name: "TerritoryInvadedByMoveEvent"},
			{ID: 3035, Name: "DiscardPileShuffledEvent"},
			{ID: 3036, Name: "HandLimitExceededEvent"},
			{ID: 3037, Name: "SpellUsedEvent"},
			{ID: 3038, Name: "MagicGainedEvent"},
			{ID: 3039, Name: "SpellEffectStartedEvent"},
			{ID: 3040, Name: "SpellEffectEndedEvent"},
			{ID: 3041, Name: "SpellCreatureDamagedEvent"},
			{ID: 3042, Name: "BoardCreatureDestroyedEvent"},
			{ID: 3043, Name: "TollCreatureEffectActivatedEvent"},
			{ID: 3044, Name: "InvadingCreatureDestroyedEvent"},
			{ID: 3045, Name: "DefendingCreatureDestroyedEvent"},
			{ID: 3046, Name: "DestructionCreatureEffectActivatedEvent"},
			{ID: 3047, Name: "CardReturnedToHandEvent"},
			{ID: 3048, Name: "MagicReceivedEvent"},
			{ID: 3049, Name: "LapCreatureHitPointsRecoveredEvent"},
		}},
	},
	UniqueKeyTypes: []NamedTypeDef{
		{ID: 1000, Name: "operator.email"},
		{ID: 1001, Name: "operator.subject"},
		{ID: 1002, Name: "card.name"},
		{ID: 1003, Name: "preset_deck.name"},
		{ID: 1005, Name: "player.subject"},
		{ID: 1006, Name: "starter_grant.player"},
		{ID: 1007, Name: "collection.player"},
		{ID: 1008, Name: "deck.starter"},
		{ID: 1009, Name: "deck.name"},
		{ID: 1010, Name: "game_rules.singleton"},
		{ID: 1011, Name: "grant_deck.name"},
		{ID: 1012, Name: "avatar.name"},
		{ID: 1013, Name: "board.name"},
		{ID: 1014, Name: "studio_rules.singleton"},
		{ID: 1015, Name: "npc_avatar.name"},
		{ID: 1016, Name: "npc_deck.name"},
		{ID: 1017, Name: "starter_set.name"},
		{ID: 1018, Name: "unlock.player"},
	},
	ReferenceTypes: []NamedTypeDef{
		{ID: 1000, Name: "preset_deck.card"},
		{ID: 1002, Name: "grant_deck.card"},
		{ID: 1003, Name: "npc_deck.card"},
		{ID: 1004, Name: "npc_avatar.deck"},
		{ID: 1005, Name: "starter_set.grant_deck"},
		{ID: 1006, Name: "starter_set.board"},
		{ID: 1007, Name: "starter_set.npc_avatar"},
	},
	QuotaTypes: []NamedTypeDef{
		{ID: 1000, Name: "deck.per_player"},
		{ID: 1002, Name: "avatar.per_player"},
		{ID: 1003, Name: "starter_set.published"},
	},
}

type Catalog struct {
	EntityTypes    []EntityTypeDef
	UniqueKeyTypes []NamedTypeDef
	ReferenceTypes []NamedTypeDef
	QuotaTypes     []NamedTypeDef
}

type EntityTypeDef struct {
	ID     int16
	Name   string
	Events []EventTypeDef
}

type EventTypeDef struct {
	ID   int16
	Name string
}

type NamedTypeDef struct {
	ID   int16
	Name string
}

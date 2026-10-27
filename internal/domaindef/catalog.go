package domaindef

// 種別の名前と id の一覧（全サービスで共通。core_studio・core_game のどちらの DB にも、同じ行を入れる）。
// id は自動で振らない（DB ごと・起動の順番で変わりうるため）。一度決めた名前と id は変えない・消さない（過去のイベントが id で
// 参照しているため）。ただし main に入る前（リリース前）は除く（core#42 でスターターデッキセットの種別を、開発の DB を作り直す前提で消した）。
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
		}},
		{ID: 1200, Name: "item", Events: []EventTypeDef{
			{ID: 1201, Name: "RegisteredEvent"},
			{ID: 1202, Name: "PublishedEvent"},
			{ID: 1203, Name: "RetiredEvent"},
		}},
		{ID: 1300, Name: "spell", Events: []EventTypeDef{
			{ID: 1301, Name: "RegisteredEvent"},
			{ID: 1302, Name: "PublishedEvent"},
			{ID: 1303, Name: "RetiredEvent"},
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
		}},
		{ID: 1800, Name: "collection", Events: []EventTypeDef{
			{ID: 1801, Name: "CreatedEvent"},
		}},
		{ID: 1900, Name: "deck", Events: []EventTypeDef{
			{ID: 1901, Name: "CreatedEvent"},
		}},
		{ID: 2000, Name: "game_rules", Events: []EventTypeDef{
			{ID: 2001, Name: "CreatedEvent"},
		}},
		{ID: 2100, Name: "starter_deck", Events: []EventTypeDef{
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
		}},
		{ID: 2400, Name: "studio_rules", Events: []EventTypeDef{
			{ID: 2401, Name: "CreatedEvent"},
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
		{ID: 1011, Name: "starter_deck.name"},
		{ID: 1012, Name: "avatar.name"},
		{ID: 1013, Name: "board.name"},
		{ID: 1014, Name: "studio_rules.singleton"},
	},
	ReferenceTypes: []NamedTypeDef{
		{ID: 1000, Name: "preset_deck.card"},
		{ID: 1002, Name: "starter_deck.card"},
	},
	QuotaTypes: []NamedTypeDef{
		{ID: 1000, Name: "deck.per_player"},
		{ID: 1001, Name: "starter_deck.published"},
		{ID: 1002, Name: "avatar.per_player"},
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

package domaindef

// 種別の名前と id の一覧（全サービスで共通。core_studio・core_game のどちらの DB にも、同じ行を入れる）。
// id は自動で振らない（DB ごと・起動の順番で変わりうるため）。一度決めた名前と id は変えない・消さない（過去のイベントが id で
// 参照しているため）。名前は各集約の宣言（Types() 等）にも書くが、ドメインのコードには id を持たせない（保存の都合のため）。
// この一覧と DB は Sync が合わせ、DB と各集約の宣言は Verify... が照合する（core#33）
//
// 振り方: 集約は 1000 から 100 刻み、イベントはその集約の id ＋ 1 から順に、一意性の key・参照は 1000 から順に。
// テスト用の範囲は持たない（偽の種別を足すテストは複製した DB を使う）
var Types = Catalog{
	EntityTypes: []EntityTypeDef{
		{ID: 1000, Name: "operator", Events: []EventTypeDef{
			{ID: 1001, Name: "RegisteredEvent"},
			{ID: 1002, Name: "LinkedEvent"},
		}},
		{ID: 1100, Name: "creature", Events: []EventTypeDef{
			{ID: 1101, Name: "RegisteredEvent"},
		}},
		{ID: 1200, Name: "item", Events: []EventTypeDef{
			{ID: 1201, Name: "RegisteredEvent"},
		}},
		{ID: 1300, Name: "spell", Events: []EventTypeDef{
			{ID: 1301, Name: "RegisteredEvent"},
		}},
		{ID: 1400, Name: "preset_deck", Events: []EventTypeDef{
			{ID: 1401, Name: "RegisteredEvent"},
			{ID: 1402, Name: "RenamedEvent"},
			{ID: 1403, Name: "CardCountsChangedEvent"},
			{ID: 1404, Name: "DeletedEvent"},
		}},
		{ID: 1500, Name: "starter_deck_set", Events: []EventTypeDef{
			{ID: 1501, Name: "DecksSetEvent"},
		}},
		{ID: 1600, Name: "player", Events: []EventTypeDef{
			{ID: 1601, Name: "RegisteredEvent"},
		}},
	},
	UniqueKeyTypes: []NamedTypeDef{
		{ID: 1000, Name: "operator.email"},
		{ID: 1001, Name: "operator.subject"},
		{ID: 1002, Name: "card.name"},
		{ID: 1003, Name: "preset_deck.name"},
		{ID: 1004, Name: "starter_deck_set.singleton"},
		{ID: 1005, Name: "player.subject"},
	},
	ReferenceTypes: []NamedTypeDef{
		{ID: 1000, Name: "preset_deck.card"},
		{ID: 1001, Name: "starter_deck_set.card"},
	},
}

type Catalog struct {
	EntityTypes    []EntityTypeDef
	UniqueKeyTypes []NamedTypeDef
	ReferenceTypes []NamedTypeDef
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

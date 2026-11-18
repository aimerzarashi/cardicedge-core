package testhelper

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 見本のマスタの配布デッキ。火は A が 4 枚・B が 3 枚、水は A が 2 枚・B が 3 枚・C が 2 枚（どちらも 7 枚。本番も公開された配布デッキの合計は決まりの枚数とちょうど同じなので、見本の game_rules は 7 枚にしてある）
// （所持カードは、カードごとに多いほうの枚数になる: A が 4 枚・B が 3 枚・C が 2 枚）。
// 準備中・使用不可の配布デッキも写してあり（C を 5 枚持つので、配られれば所持カードが変わる）、配られないことを確かめる。
// 公開中のスターターセットは、配るデッキを 水・使用不可・火 の順に指す（配られるのは公開中の 水 → 火 の順。名前の順（火 → 水）でも
// 配布デッキの行の順でもなく、セットが持つ順に配られることと、公開中でないデッキが飛ばされることを確かめる）。
// 解放するボードは 公開中B・使用不可・公開中A の順（解放されて見えるのは 公開中B → 公開中A）
type Master struct {
	CardA, CardB, CardC uuid.UUID
	// game の内部 ID で指したカード（make publish-master が game の domain_entities に作る行。種類はどれも Creature）
	InternalCardA, InternalCardB, InternalCardC card.Ref
	GrantDecks                                  GrantDecks
	// PublishMaster と PublishMasterWithoutGameRules が写す
	Boards Boards
	// PublishMaster と PublishMasterWithoutGameRules が写す
	StarterSets StarterSets
	// PublishMaster と PublishMasterWithoutGameRules が写す
	NPCAvatars NPCAvatars
}

type GrantDecks struct {
	Fire, Water, Draft, Retired uuid.UUID
}

// 公開中のセットが 1 つ、公開中でないものが 2 つ（公開中でないセットは火だけを配る設定。写されても使われないことを確かめる）
type StarterSets struct {
	Published, Draft, Retired uuid.UUID
}

// 見本のマスタの NPC のアバターと NPC のデッキ。スターターセットが解放する公開中のアバターが 2 つ（A・B）、公開中でないものが 2 つ、
// 公開中だが解放されないものが 1 つ。
// 公開中の A は、割り当てた順に 公開中のデッキ 2・準備中・公開中のデッキ 1・使用不可 を使う（公開中でないデッキは出ない）。
// B は公開中のデッキ 1 だけを使う（同じデッキを複数のアバターが使える）。
// make publish-master は割り当てられていないデッキを写さないが、写されても出ないことを確かめるために、どのアバターにも使われない公開中のデッキを入れる
type NPCAvatars struct {
	PublishedA, PublishedB, Draft, Retired uuid.UUID
	// 公開中だが、スターターセットが解放しないアバター（解放していないものは出ないことを確かめる）
	PublishedLocked uuid.UUID
	// game の内部 ID（make publish-master が game の domain_entities に作る行の ID）
	InternalPublishedA, InternalPublishedB, InternalRetired npcavatar.ID
	DeckPublished1, DeckPublished2                          uuid.UUID
	DeckDraft, DeckRetired, DeckUnassigned                  uuid.UUID
	// アバターに割り当てた思考パターン。A は割り当てた順に 公開中 1・準備中・公開中 2・使用不可 を使い（公開中でないものは出ない）、
	// B は公開中 1 だけを使う（同じ思考パターンを複数のアバターが使える）
	MindsetPublished1, MindsetPublished2, MindsetDraft, MindsetRetired uuid.UUID
	// 公開中だが、どのアバターにも割り当てていない思考パターン（割り当てていないものは選べないことを確かめる）
	MindsetUnassigned uuid.UUID
	// game の内部 ID（make publish-master が game の domain_entities に作る行の ID。部屋の席の選択が指す ID）
	InternalMindsetPublished1 npcmindset.ID
	// 公開中のデッキ 1 に入れたカード（種類は順に Creature・Item・Spell。名前の順は Item のほうが先だが、種類が優先される）
	Creature, Item, Spell uuid.UUID
}

// 見本のマスタのボード。公開中が 2 つ（名前の順に A・B）、公開中でないものが 2 つ。
// 準備中は make publish-master が写さないが、写されても出ないことを確かめるために入れる（配布デッキの準備中と同じ）
type Boards struct {
	PublishedA, PublishedB, Draft, Retired uuid.UUID
	// game の内部 ID（make publish-master が game の domain_entities に作る行の ID。game の集約がボードを指す ID）
	InternalPublishedA, InternalPublishedB, InternalRetired board.ID
}

// 公開は、studio の read model の cards・grant_decks・starter_sets・game_rules の行を game の DB に写すことなので（make publish-master）、
// 行を直接入れる。metadata は書き手（studio の read model）の型で作り、読み手と同じ形であることも確かめる。
// 行を入れるのは、公開されていない状態から公開された状態への変わり目を確かめる（登録が先、公開が後）ため、アプリの起動とは別にしてある
func PublishMaster(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := PublishMasterWithoutGameRules(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := PublishGameRules(ctx, gameDB); err != nil {
		return Master{}, err
	}
	return master, nil
}

// カード・配布デッキ・ボード・NPC のアバター・スターターセットだけを写す（game_rules を写し忘れた状態を確かめるため）
func PublishMasterWithoutGameRules(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := publishCards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := publishGrantDecks(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	master.Boards, err = PublishBoards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	master.NPCAvatars, err = PublishNPCAvatars(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := publishUnpublishedStarterSets(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	return master, PublishStarterSet(ctx, gameDB, master)
}

// 公開中のスターターセットが 1 つも無い状態で写す（カード・配布デッキ・ボード・準備中と使用不可のスターターセット・game_rules。
// 公開中が無ければ配布が進まないことを確かめるため）。公開中のものは、後から PublishStarterSet で足せる
func PublishMasterWithoutPublishedStarterSet(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := publishCards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := publishGrantDecks(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	master.Boards, err = PublishBoards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	master.NPCAvatars, err = PublishNPCAvatars(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := publishUnpublishedStarterSets(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	return master, PublishGameRules(ctx, gameDB)
}

func publishCards(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master := Master{
		CardA: uuid.New(), CardB: uuid.New(), CardC: uuid.New(),
		GrantDecks:  GrantDecks{Fire: uuid.New(), Water: uuid.New(), Draft: uuid.New(), Retired: uuid.New()},
		StarterSets: StarterSets{Published: uuid.New(), Draft: uuid.New(), Retired: uuid.New()},
	}
	for _, id := range []uuid.UUID{master.CardA, master.CardB, master.CardC} {
		metadata, err := json.Marshal(readmodel.CardMetadata{ID: id, Name: id.String(), Cost: 10, Rarity: "normal", Creature: &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Effects: []readmodel.EffectMetadata{}}})
		if err != nil {
			return Master{}, err
		}
		// 公開中にするのは、戦利品の候補（公開中のカード）になるため。make publish-master は準備中を写さない
		card := models.Card{CardID: id.String(), CardType: int16(readmodel.CardTypeCreature), SortName: id.String(), Metadata: metadata, Status: "published"}
		if err := card.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return Master{}, err
		}
	}
	if _, err := gameDB.ExecContext(ctx, publishCardEntitiesSQL); err != nil {
		return Master{}, err
	}
	for public, internal := range map[uuid.UUID]*card.Ref{
		master.CardA: &master.InternalCardA, master.CardB: &master.InternalCardB, master.CardC: &master.InternalCardC,
	} {
		ref, err := InternalCardRef(ctx, gameDB, public)
		if err != nil {
			return Master{}, err
		}
		*internal = ref
	}
	return master, nil
}

// Makefile の publish-master が、写した cards について流す文と同じ（公開 ID は studio と同じ、内部 ID は game の連番。
// 種類は写した cards.card_type から決める）
const publishCardEntitiesSQL = `INSERT INTO domain_entities (public_id, entity_type_id) SELECT c.card_id, t.id FROM cards c JOIN domain_entity_types t ON t.name = CASE c.card_type WHEN 1 THEN 'creature' WHEN 2 THEN 'item' WHEN 3 THEN 'spell' END ON CONFLICT DO NOTHING`

// 写したカードの公開 ID を、game の内部 ID で指す card.Ref に引く
func InternalCardRef(ctx context.Context, gameDB *sql.DB, publicID uuid.UUID) (card.Ref, error) {
	var value int64
	var entityType string
	err := gameDB.QueryRowContext(ctx,
		`SELECT e.id, t.name FROM domain_entities e JOIN domain_entity_types t ON t.id = e.entity_type_id WHERE e.public_id = $1`,
		publicID).Scan(&value, &entityType)
	if err != nil {
		return card.Ref{}, err
	}
	return card.NewRef(domain.NewEntityType(entityType), value)
}

// 配布デッキ 4 つ（公開中が火と水、準備中、使用不可）を写す。C を 5 枚持つのが準備中と使用不可なので、配られれば所持カードが変わる
func publishGrantDecks(ctx context.Context, gameDB *sql.DB, master Master) error {
	fire := readmodel.GrantDeckMetadata{ID: master.GrantDecks.Fire, Name: "火", Status: readmodel.GrantDeckStatusPublished, Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 4}, {CardID: master.CardB, Count: 3}}}
	water := readmodel.GrantDeckMetadata{ID: master.GrantDecks.Water, Name: "水", Status: readmodel.GrantDeckStatusPublished, Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 2}, {CardID: master.CardB, Count: 3}, {CardID: master.CardC, Count: 2}}}
	draft := readmodel.GrantDeckMetadata{ID: master.GrantDecks.Draft, Name: "準備中", Status: readmodel.GrantDeckStatusDraft, Cards: []readmodel.DeckCard{{CardID: master.CardC, Count: 5}}}
	retired := readmodel.GrantDeckMetadata{ID: master.GrantDecks.Retired, Name: "使用不可", Status: readmodel.GrantDeckStatusRetired, Cards: []readmodel.DeckCard{{CardID: master.CardC, Count: 5}}}
	// 1 つのトランザクションで書く（make publish-master と同じ）。sort_name は並びを決めるだけの値
	tx, err := gameDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, metadata := range []readmodel.GrantDeckMetadata{fire, water, draft, retired} {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		row := models.GrantDeck{GrantDeckID: metadata.ID.String(), SortName: fmt.Sprint(i + 1), Status: string(metadata.Status), Metadata: raw}
		if err := row.Insert(ctx, tx, boil.Infer()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// 公開中のスターターセットを写す。配るデッキは 水・使用不可・火 の順、解放するボードは 公開中B・使用不可・公開中A の順、解放する NPC のアバターも 公開中B・使用不可・公開中A の順
func PublishStarterSet(ctx context.Context, gameDB *sql.DB, master Master) error {
	published := readmodel.StarterSetMetadata{
		ID: master.StarterSets.Published, Name: "公開中", Status: readmodel.StarterSetStatusPublished,
		GrantDecks:   []uuid.UUID{master.GrantDecks.Water, master.GrantDecks.Retired, master.GrantDecks.Fire},
		UnlockBoards: []uuid.UUID{master.Boards.PublishedB, master.Boards.Retired, master.Boards.PublishedA},
		// 解放する NPC のアバターも、名前の順でも行の順でもなく、セットが持つ順に解放され、公開中でないものが飛ばされることを確かめる
		UnlockNPCAvatars: []uuid.UUID{master.NPCAvatars.PublishedB, master.NPCAvatars.Retired, master.NPCAvatars.PublishedA},
	}
	return insertStarterSets(ctx, gameDB, published)
}

// 公開中でないスターターセット（準備中・使用不可）を写す。火だけを配ってボードを解放する設定で、使われれば配られる中身が変わる
func publishUnpublishedStarterSets(ctx context.Context, gameDB *sql.DB, master Master) error {
	draft := readmodel.StarterSetMetadata{
		ID: master.StarterSets.Draft, Name: "準備中", Status: readmodel.StarterSetStatusDraft,
		GrantDecks: []uuid.UUID{master.GrantDecks.Fire}, UnlockBoards: []uuid.UUID{master.Boards.PublishedA}, UnlockNPCAvatars: []uuid.UUID{master.NPCAvatars.PublishedA},
	}
	retired := readmodel.StarterSetMetadata{
		ID: master.StarterSets.Retired, Name: "使用不可", Status: readmodel.StarterSetStatusRetired,
		GrantDecks: []uuid.UUID{master.GrantDecks.Fire}, UnlockBoards: []uuid.UUID{master.Boards.PublishedA}, UnlockNPCAvatars: []uuid.UUID{master.NPCAvatars.PublishedA},
	}
	return insertStarterSets(ctx, gameDB, draft, retired)
}

func insertStarterSets(ctx context.Context, gameDB *sql.DB, sets ...readmodel.StarterSetMetadata) error {
	for _, metadata := range sets {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		row := models.StarterSet{StarterSetID: metadata.ID.String(), SortName: metadata.ID.String(), Status: string(metadata.Status), Metadata: raw}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return err
		}
	}
	return nil
}

// game_rules の行を写す。値は既定の値（studio が起動時に作る値）だが、デッキの枚数だけは見本の配布デッキの合計（7 枚）にする
func PublishGameRules(ctx context.Context, gameDB *sql.DB) error {
	rules := gamerules.Defaults()
	total, err := card.NewDeckTotalCount(7)
	if err != nil {
		return err
	}
	rules.Deck.TotalCardCount = total
	metadata, err := json.Marshal(readmodel.GameRulesMetadata{ID: uuid.New(), Revision: 1, Rules: rules})
	if err != nil {
		return err
	}
	row := models.GameRule{GameRulesID: uuid.NewString(), Metadata: metadata}
	return row.Insert(ctx, gameDB, boil.Infer())
}

// boards の行を写す。公開中の A は 2×1（拠点とチェックポイントがつながる。既定のルールは登録の直後の値）、B は 3×2（真ん中のマスが 3 方向につながる。転送円の対がある。既定のルールは A と違う値）
func PublishBoards(ctx context.Context, gameDB *sql.DB) (Boards, error) {
	boards := Boards{PublishedA: uuid.New(), PublishedB: uuid.New(), Draft: uuid.New(), Retired: uuid.New()}
	a := readmodel.BoardMetadata{
		ID: boards.PublishedA, Name: "公開中A", Status: readmodel.BoardStatusPublished, Width: 2, Height: 1,
		Tiles: []readmodel.BoardTile{
			{X: 0, Y: 0, Kind: readmodel.BoardTileKindBase},
			{X: 1, Y: 0, Kind: readmodel.BoardTileKindCheckpoint},
		},
		Connections: []readmodel.BoardConnection{
			{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)},
		},
		Pairs:        []readmodel.BoardPair{},
		DefaultRules: readmodel.NewBoardDefaultRules(8000, 600, 300, 100, 30, 1, 6, 100, 20),
		TileCounts:   readmodel.NewBoardTileCounts(0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
	}
	b := readmodel.BoardMetadata{
		ID: boards.PublishedB, Name: "公開中B", Status: readmodel.BoardStatusPublished, Width: 3, Height: 2,
		Tiles: []readmodel.BoardTile{
			{X: 0, Y: 0, Kind: readmodel.BoardTileKindBase},
			{X: 1, Y: 0, Kind: readmodel.BoardTileKindLand, Element: readmodel.BoardTileElementFire},
			{X: 2, Y: 0, Kind: readmodel.BoardTileKindCheckpoint},
			{X: 0, Y: 1, Kind: readmodel.BoardTileKindWarp},
			{X: 1, Y: 1, Kind: readmodel.BoardTileKindLand, Element: readmodel.BoardTileElementMulti},
			{X: 2, Y: 1, Kind: readmodel.BoardTileKindWarp},
		},
		Connections: []readmodel.BoardConnection{
			{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)},
			{A: readmodel.NewBoardPosition(1, 0), B: readmodel.NewBoardPosition(1, 1)},
			{A: readmodel.NewBoardPosition(1, 0), B: readmodel.NewBoardPosition(2, 0)},
			{A: readmodel.NewBoardPosition(0, 1), B: readmodel.NewBoardPosition(1, 1)},
		},
		Pairs:        []readmodel.BoardPair{{A: readmodel.NewBoardPosition(0, 1), B: readmodel.NewBoardPosition(2, 1)}},
		DefaultRules: readmodel.NewBoardDefaultRules(5000, 0, 200, 50, 12, 2, 8, 100, 40),
		TileCounts:   readmodel.NewBoardTileCounts(2, 1, 1, 1, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 0),
	}
	// 公開中でなければ出ないことだけを確かめるので、中身は A と同じにする
	draft, retired := a, a
	draft.ID, draft.Name, draft.Status = boards.Draft, "準備中", readmodel.BoardStatusDraft
	retired.ID, retired.Name, retired.Status = boards.Retired, "使用不可", readmodel.BoardStatusRetired
	// sort_name は並びを決めるだけの値（読み手は sort_name の順に返す）
	for i, metadata := range []readmodel.BoardMetadata{a, b, draft, retired} {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return Boards{}, err
		}
		row := models.Board{BoardID: metadata.ID.String(), SortName: fmt.Sprint(i + 1), Status: string(metadata.Status), Metadata: raw}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return Boards{}, err
		}
	}
	// make publish-master と同じ文で、写したボードの game の内部 ID（domain_entities の行）を作る
	if _, err := gameDB.ExecContext(ctx, publishBoardEntitiesSQL); err != nil {
		return Boards{}, err
	}
	for publicID, internalID := range map[uuid.UUID]*board.ID{
		boards.PublishedA: &boards.InternalPublishedA, boards.PublishedB: &boards.InternalPublishedB, boards.Retired: &boards.InternalRetired,
	} {
		var value int64
		if err := gameDB.QueryRowContext(ctx, `SELECT id FROM domain_entities WHERE public_id = $1`, publicID).Scan(&value); err != nil {
			return Boards{}, err
		}
		id, err := board.NewID(value)
		if err != nil {
			return Boards{}, err
		}
		*internalID = id
	}
	return boards, nil
}

// Makefile の publish-master が、写した boards について流す文と同じ（公開 ID は studio と同じ、内部 ID は game の連番）
const publishBoardEntitiesSQL = `INSERT INTO domain_entities (public_id, entity_type_id) SELECT b.board_id, t.id FROM boards b JOIN domain_entity_types t ON t.name = 'board' ON CONFLICT DO NOTHING`

// npc_avatars・npc_decks と、デッキが入れるカードの行を写す。カードは公開中のデッキ 1 が入れる 3 種類（種類が違う）。
// デッキ 1 は書き手の並びを Spell・Creature・Item にして、読み手が種類の順に並べ直すことを確かめる
func PublishNPCAvatars(ctx context.Context, gameDB *sql.DB) (NPCAvatars, error) {
	n := NPCAvatars{
		PublishedA: uuid.New(), PublishedB: uuid.New(), Draft: uuid.New(), Retired: uuid.New(), PublishedLocked: uuid.New(),
		DeckPublished1: uuid.New(), DeckPublished2: uuid.New(), DeckDraft: uuid.New(), DeckRetired: uuid.New(), DeckUnassigned: uuid.New(),
		MindsetPublished1: uuid.New(), MindsetPublished2: uuid.New(), MindsetDraft: uuid.New(), MindsetRetired: uuid.New(), MindsetUnassigned: uuid.New(),
		Creature: uuid.New(), Item: uuid.New(), Spell: uuid.New(),
	}
	// 性能は種類ごとに変え、写した値がそのカードのものかを確かめられるようにする
	cards := []struct {
		id       uuid.UUID
		cardType readmodel.CardType
		name     string
		cost     int
		rarity   string
	}{
		{n.Creature, readmodel.CardTypeCreature, "ゴブリン", 20, "normal"},
		{n.Item, readmodel.CardTypeItem, "あいてむ", 30, "strange"},
		{n.Spell, readmodel.CardTypeSpell, "ファイア", 40, "rare"},
	}
	for _, c := range cards {
		cardMetadata := readmodel.CardMetadata{ID: c.id, Name: c.name, Status: readmodel.CardStatusPublished, Cost: c.cost, Rarity: c.rarity}
		if c.cardType == readmodel.CardTypeCreature {
			cardMetadata.Creature = &readmodel.CreatureMetadata{Elements: []string{"fire", "wind"}, Attack: 25, HitPoints: 45, ItemRestrictions: []string{"armor", "scroll"}, PlacementRestrictions: []string{"earth", "wind"}, Effects: []readmodel.EffectMetadata{{ForbidInvasion: &readmodel.EmptyEffectMetadata{}}, {ForbidMovement: &readmodel.EmptyEffectMetadata{}}}}
		}
		if c.cardType == readmodel.CardTypeItem {
			cardMetadata.Item = &readmodel.ItemMetadata{Category: "armor", Effects: []readmodel.EffectMetadata{{HitPointsModifier: &readmodel.ValueEffectMetadata{Value: 15}}}}
		}
		if c.cardType == readmodel.CardTypeSpell {
			effect := readmodel.SpellEffectMetadata{DrawCards: &readmodel.SpellDrawCardsMetadata{Count: 2}}
			cardMetadata.Spell = &readmodel.SpellMetadata{Effect: effect}
		}
		metadata, err := json.Marshal(cardMetadata)
		if err != nil {
			return NPCAvatars{}, err
		}
		row := models.Card{CardID: c.id.String(), CardType: int16(c.cardType), SortName: c.name, Status: string(readmodel.CardStatusPublished), Metadata: metadata}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return NPCAvatars{}, err
		}
	}
	if _, err := gameDB.ExecContext(ctx, publishCardEntitiesSQL); err != nil {
		return NPCAvatars{}, err
	}
	// sort_name は並びを決めるだけの値（読み手は sort_name の順に返す）
	deck1 := npcDeckMetadata(n.DeckPublished1, "公開中デッキ1", readmodel.NPCDeckStatusPublished, []readmodel.DeckCard{
		{CardID: n.Spell, Count: 1}, {CardID: n.Creature, Count: 3}, {CardID: n.Item, Count: 2},
	})
	one := []readmodel.DeckCard{{CardID: n.Creature, Count: 1}}
	decks := []readmodel.NPCDeckMetadata{
		deck1,
		npcDeckMetadata(n.DeckPublished2, "公開中デッキ2", readmodel.NPCDeckStatusPublished, []readmodel.DeckCard{{CardID: n.Creature, Count: 4}}),
		npcDeckMetadata(n.DeckDraft, "準備中デッキ", readmodel.NPCDeckStatusDraft, one),
		npcDeckMetadata(n.DeckRetired, "使用不可デッキ", readmodel.NPCDeckStatusRetired, one),
		npcDeckMetadata(n.DeckUnassigned, "割り当てなしデッキ", readmodel.NPCDeckStatusPublished, one),
	}
	for i, metadata := range decks {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return NPCAvatars{}, err
		}
		row := models.NPCDeck{NPCDeckID: metadata.ID.String(), SortName: fmt.Sprint(i + 1), Status: string(metadata.Status), Metadata: raw}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return NPCAvatars{}, err
		}
	}
	mindsets := []readmodel.NPCMindsetMetadata{
		npcMindsetMetadata(n.MindsetPublished1, "公開中思考パターン1", readmodel.NPCMindsetStatusPublished, 2, 3),
		npcMindsetMetadata(n.MindsetPublished2, "公開中思考パターン2", readmodel.NPCMindsetStatusPublished, 1, 1),
		npcMindsetMetadata(n.MindsetDraft, "準備中思考パターン", readmodel.NPCMindsetStatusDraft, 1, 1),
		npcMindsetMetadata(n.MindsetRetired, "使用不可思考パターン", readmodel.NPCMindsetStatusRetired, 1, 1),
		npcMindsetMetadata(n.MindsetUnassigned, "割り当てなし思考パターン", readmodel.NPCMindsetStatusPublished, 1, 1),
	}
	for i, metadata := range mindsets {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return NPCAvatars{}, err
		}
		row := models.NPCMindset{NPCMindsetID: metadata.ID.String(), SortName: fmt.Sprint(i + 1), Status: string(metadata.Status), Metadata: raw}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return NPCAvatars{}, err
		}
	}
	avatars := []readmodel.NPCAvatarMetadata{
		npcAvatarMetadata(n.PublishedA, "公開中A", readmodel.NPCAvatarStatusPublished, []uuid.UUID{n.DeckPublished2, n.DeckDraft, n.DeckPublished1, n.DeckRetired}, n.MindsetPublished1, n.MindsetDraft, n.MindsetPublished2, n.MindsetRetired),
		npcAvatarMetadata(n.PublishedB, "公開中B", readmodel.NPCAvatarStatusPublished, []uuid.UUID{n.DeckPublished1}, n.MindsetPublished1),
		// 公開中でなければ出ないことだけを確かめるので、使うデッキは公開中のものにする
		npcAvatarMetadata(n.Draft, "準備中", readmodel.NPCAvatarStatusDraft, []uuid.UUID{n.DeckPublished1}),
		npcAvatarMetadata(n.Retired, "使用不可", readmodel.NPCAvatarStatusRetired, []uuid.UUID{n.DeckPublished1}),
		npcAvatarMetadata(n.PublishedLocked, "未解放", readmodel.NPCAvatarStatusPublished, []uuid.UUID{n.DeckPublished1}),
	}
	for i, metadata := range avatars {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return NPCAvatars{}, err
		}
		row := models.NPCAvatar{NPCAvatarID: metadata.ID.String(), SortName: fmt.Sprint(i + 1), Status: string(metadata.Status), Metadata: raw}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return NPCAvatars{}, err
		}
	}
	// make publish-master と同じ文で、写した NPC のアバターとデッキの game の内部 ID（domain_entities の行）を作る
	if _, err := gameDB.ExecContext(ctx, publishNPCAvatarEntitiesSQL); err != nil {
		return NPCAvatars{}, err
	}
	if _, err := gameDB.ExecContext(ctx, publishNPCDeckEntitiesSQL); err != nil {
		return NPCAvatars{}, err
	}
	if _, err := gameDB.ExecContext(ctx, publishNPCMindsetEntitiesSQL); err != nil {
		return NPCAvatars{}, err
	}
	var mindsetValue int64
	if err := gameDB.QueryRowContext(ctx, `SELECT id FROM domain_entities WHERE public_id = $1`, n.MindsetPublished1).Scan(&mindsetValue); err != nil {
		return NPCAvatars{}, err
	}
	mindsetID, err := npcmindset.NewID(mindsetValue)
	if err != nil {
		return NPCAvatars{}, err
	}
	n.InternalMindsetPublished1 = mindsetID
	for publicID, internalID := range map[uuid.UUID]*npcavatar.ID{
		n.PublishedA: &n.InternalPublishedA, n.PublishedB: &n.InternalPublishedB, n.Retired: &n.InternalRetired,
	} {
		var value int64
		if err := gameDB.QueryRowContext(ctx, `SELECT id FROM domain_entities WHERE public_id = $1`, publicID).Scan(&value); err != nil {
			return NPCAvatars{}, err
		}
		id, err := npcavatar.NewID(value)
		if err != nil {
			return NPCAvatars{}, err
		}
		*internalID = id
	}
	return n, nil
}

// Makefile の publish-master が、写した npc_avatars について流す文と同じ
const publishNPCAvatarEntitiesSQL = `INSERT INTO domain_entities (public_id, entity_type_id) SELECT a.npc_avatar_id, t.id FROM npc_avatars a JOIN domain_entity_types t ON t.name = 'npc_avatar' ON CONFLICT DO NOTHING`

// Makefile の publish-master が、写した npc_decks について流す文と同じ（部屋の席の選択が NPC のデッキを game の内部 ID で指すため）
const publishNPCDeckEntitiesSQL = `INSERT INTO domain_entities (public_id, entity_type_id) SELECT d.npc_deck_id, t.id FROM npc_decks d JOIN domain_entity_types t ON t.name = 'npc_deck' ON CONFLICT DO NOTHING`

// Makefile の publish-master が、写した npc_mindsets について流す文と同じ（部屋の席の選択が思考パターンを game の内部 ID で指すため）
const publishNPCMindsetEntitiesSQL = `INSERT INTO domain_entities (public_id, entity_type_id) SELECT m.npc_mindset_id, t.id FROM npc_mindsets m JOIN domain_entity_types t ON t.name = 'npc_mindset' ON CONFLICT DO NOTHING`

// 行動の種類ごとの重みはどれも actionWeight、採点基準ごとの重みは総魔力の差だけ criterionWeight で、ほかは 1（写した値が読み手に届くことを確かめる）
func npcMindsetMetadata(id uuid.UUID, name string, status readmodel.NPCMindsetStatus, actionWeight, criterionWeight int) readmodel.NPCMindsetMetadata {
	metadata := readmodel.NPCMindsetMetadata{
		ID: id, Name: name, Status: status, Revision: 2,
		ActionWeights: map[matchaction.Action]int{}, CriterionWeights: map[npcmindset.Criterion]int{},
	}
	for _, action := range matchaction.Actions() {
		metadata.ActionWeights[action] = actionWeight
	}
	for _, criterion := range npcmindset.Criteria() {
		metadata.CriterionWeights[criterion] = 1
	}
	metadata.CriterionWeights[npcmindset.CriterionTotalMagicLead] = criterionWeight
	return metadata
}

func npcDeckMetadata(id uuid.UUID, name string, status readmodel.NPCDeckStatus, cards []readmodel.DeckCard) readmodel.NPCDeckMetadata {
	return readmodel.NPCDeckMetadata{ID: id, Name: name, Status: status, Cards: cards}
}

func npcAvatarMetadata(id uuid.UUID, name string, status readmodel.NPCAvatarStatus, decks []uuid.UUID, mindsets ...uuid.UUID) readmodel.NPCAvatarMetadata {
	return readmodel.NPCAvatarMetadata{ID: id, Name: name, Status: status, Decks: decks, Mindsets: append([]uuid.UUID{}, mindsets...)}
}

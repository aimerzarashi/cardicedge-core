package testhelper

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 見本のマスタの公開中の配布デッキ。デッキ 1 は A が 4 枚・B が 1 枚、デッキ 2 は A が 2 枚・B が 3 枚・C が 2 枚
// （所持カードは、カードごとに多いほうの枚数になる: A が 4 枚・B が 3 枚・C が 2 枚）。
// 準備中・使用不可の配布デッキも写してあり（C を 5 枚持つので、配られれば所持カードが変わる）、配られないことを確かめる
type Master struct {
	CardA, CardB, CardC uuid.UUID
	// PublishMaster だけが写す
	Boards Boards
	// PublishMaster だけが写す
	NPCAvatars NPCAvatars
}

// 見本のマスタの NPC のアバターと NPC のデッキ。公開中のアバターが 2 つ（名前の順に A・B）、公開中でないものが 2 つ。
// 公開中の A は、割り当てた順に 公開中のデッキ 2・準備中・公開中のデッキ 1・使用不可 を使う（公開中でないデッキは出ない）。
// B は公開中のデッキ 1 だけを使う（同じデッキを複数のアバターが使える）。
// make publish-master は割り当てられていないデッキを写さないが、写されても出ないことを確かめるために、どのアバターにも使われない公開中のデッキを入れる
type NPCAvatars struct {
	PublishedA, PublishedB, Draft, Retired uuid.UUID
	DeckPublished1, DeckPublished2         uuid.UUID
	DeckDraft, DeckRetired, DeckUnassigned uuid.UUID
	// 公開中のデッキ 1 に入れたカード（種類は順に Creature・Item・Spell。名前の順は Item のほうが先だが、種類が優先される）
	Creature, Item, Spell uuid.UUID
}

// 見本のマスタのボード。公開中が 2 つ（名前の順に A・B）、公開中でないものが 2 つ。
// 準備中は make publish-master が写さないが、写されても出ないことを確かめるために入れる（配布デッキの準備中と同じ）
type Boards struct {
	PublishedA, PublishedB, Draft, Retired uuid.UUID
}

// 公開は、studio の read model の cards・grant_decks・game_rules の行を game の DB に写すことなので（make publish-master）、
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
	master.Boards, err = PublishBoards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	master.NPCAvatars, err = PublishNPCAvatars(ctx, gameDB)
	return master, err
}

// カード・配布デッキだけを写す（game_rules を写し忘れた状態を確かめるため）
func PublishMasterWithoutGameRules(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := publishCards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := PublishGrantDecks(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	return master, publishUnpublishedGrantDecks(ctx, gameDB, master)
}

// 公開中の配布デッキが 1 つも無い状態で写す（カード・準備中と使用不可の配布デッキ・game_rules。
// 公開中が無ければ配布が進まないことを確かめるため）。公開中のものは、後から PublishGrantDecks で足せる
func PublishMasterWithoutPublishedGrantDecks(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := publishCards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := publishUnpublishedGrantDecks(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	return master, PublishGameRules(ctx, gameDB)
}

func publishCards(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master := Master{CardA: uuid.New(), CardB: uuid.New(), CardC: uuid.New()}
	for _, id := range []uuid.UUID{master.CardA, master.CardB, master.CardC} {
		metadata, err := json.Marshal(readmodel.CardMetadata{ID: id, Name: id.String()})
		if err != nil {
			return Master{}, err
		}
		card := models.Card{CardID: id.String(), CardType: int16(readmodel.CardTypeCreature), SortName: id.String(), Metadata: metadata}
		if err := card.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return Master{}, err
		}
	}
	return master, nil
}

// 公開中の配布デッキ 2 つを写す
func PublishGrantDecks(ctx context.Context, gameDB *sql.DB, master Master) error {
	fire := readmodel.GrantDeckMetadata{Name: "火", Status: readmodel.GrantDeckStatusPublished, Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 4}, {CardID: master.CardB, Count: 1}}}
	water := readmodel.GrantDeckMetadata{Name: "水", Status: readmodel.GrantDeckStatusPublished, Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 2}, {CardID: master.CardB, Count: 3}, {CardID: master.CardC, Count: 2}}}
	// sort_name は並びを決めるだけの値（読み手は sort_name の順に配る）
	return insertGrantDecks(ctx, gameDB, []grantDeckRow{{"1", fire}, {"2", water}})
}

// C を 5 枚持つので、配られれば所持カードが変わる
func publishUnpublishedGrantDecks(ctx context.Context, gameDB *sql.DB, master Master) error {
	draft := readmodel.GrantDeckMetadata{Name: "準備中", Status: readmodel.GrantDeckStatusDraft, Cards: []readmodel.DeckCard{{CardID: master.CardC, Count: 5}}}
	retired := readmodel.GrantDeckMetadata{Name: "使用不可", Status: readmodel.GrantDeckStatusRetired, Cards: []readmodel.DeckCard{{CardID: master.CardC, Count: 5}}}
	return insertGrantDecks(ctx, gameDB, []grantDeckRow{{"3", draft}, {"4", retired}})
}

type grantDeckRow struct {
	sortName string
	metadata readmodel.GrantDeckMetadata
}

// 1 つのトランザクションで書く（make publish-master と同じ）。行ごとに書くと、登録の後に写すテストで、配布が 1 つ目の
// デッキだけを読んで配ることがある（core#53）
func insertGrantDecks(ctx context.Context, gameDB *sql.DB, decks []grantDeckRow) error {
	tx, err := gameDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, deck := range decks {
		deck.metadata.ID = uuid.New()
		metadata, err := json.Marshal(deck.metadata)
		if err != nil {
			return err
		}
		row := models.GrantDeck{GrantDeckID: deck.metadata.ID.String(), SortName: deck.sortName, Status: string(deck.metadata.Status), Metadata: metadata}
		if err := row.Insert(ctx, tx, boil.Infer()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// game_rules の行を写す。値は既定の値（studio が起動時に作る値）
func PublishGameRules(ctx context.Context, gameDB *sql.DB) error {
	metadata, err := json.Marshal(readmodel.GameRulesMetadata{ID: uuid.New(), Revision: 1, Rules: gamerules.Defaults()})
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
		DefaultRules: readmodel.NewBoardDefaultRules(8000, 600, 300, 100, 30, 1, 6),
		TileCounts:   readmodel.NewBoardTileCounts(0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
	}
	b := readmodel.BoardMetadata{
		ID: boards.PublishedB, Name: "公開中B", Status: readmodel.BoardStatusPublished, Width: 3, Height: 2,
		Tiles: []readmodel.BoardTile{
			{X: 0, Y: 0, Kind: readmodel.BoardTileKindBase},
			{X: 1, Y: 0, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeFire},
			{X: 2, Y: 0, Kind: readmodel.BoardTileKindCheckpoint},
			{X: 0, Y: 1, Kind: readmodel.BoardTileKindWarp},
			{X: 1, Y: 1, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeMulti},
			{X: 2, Y: 1, Kind: readmodel.BoardTileKindWarp},
		},
		Connections: []readmodel.BoardConnection{
			{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)},
			{A: readmodel.NewBoardPosition(1, 0), B: readmodel.NewBoardPosition(1, 1)},
			{A: readmodel.NewBoardPosition(1, 0), B: readmodel.NewBoardPosition(2, 0)},
			{A: readmodel.NewBoardPosition(0, 1), B: readmodel.NewBoardPosition(1, 1)},
		},
		Pairs:        []readmodel.BoardPair{{A: readmodel.NewBoardPosition(0, 1), B: readmodel.NewBoardPosition(2, 1)}},
		DefaultRules: readmodel.NewBoardDefaultRules(5000, 0, 200, 50, 12, 2, 8),
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
	return boards, nil
}

// npc_avatars・npc_decks と、デッキが入れるカードの行を写す。カードは公開中のデッキ 1 が入れる 3 種類（種類が違う）。
// デッキ 1 は書き手の並びを Spell・Creature・Item にして、読み手が種類の順に並べ直すことを確かめる
func PublishNPCAvatars(ctx context.Context, gameDB *sql.DB) (NPCAvatars, error) {
	n := NPCAvatars{
		PublishedA: uuid.New(), PublishedB: uuid.New(), Draft: uuid.New(), Retired: uuid.New(),
		DeckPublished1: uuid.New(), DeckPublished2: uuid.New(), DeckDraft: uuid.New(), DeckRetired: uuid.New(), DeckUnassigned: uuid.New(),
		Creature: uuid.New(), Item: uuid.New(), Spell: uuid.New(),
	}
	cards := []struct {
		id       uuid.UUID
		cardType readmodel.CardType
		name     string
	}{
		{n.Creature, readmodel.CardTypeCreature, "ゴブリン"},
		{n.Item, readmodel.CardTypeItem, "あいてむ"},
		{n.Spell, readmodel.CardTypeSpell, "ファイア"},
	}
	for _, c := range cards {
		metadata, err := json.Marshal(readmodel.CardMetadata{ID: c.id, Name: c.name, Status: readmodel.CardStatusPublished})
		if err != nil {
			return NPCAvatars{}, err
		}
		row := models.Card{CardID: c.id.String(), CardType: int16(c.cardType), SortName: c.name, Status: string(readmodel.CardStatusPublished), Metadata: metadata}
		if err := row.Insert(ctx, gameDB, boil.Infer()); err != nil {
			return NPCAvatars{}, err
		}
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
	avatars := []readmodel.NPCAvatarMetadata{
		npcAvatarMetadata(n.PublishedA, "公開中A", readmodel.NPCAvatarStatusPublished, n.DeckPublished2, n.DeckDraft, n.DeckPublished1, n.DeckRetired),
		npcAvatarMetadata(n.PublishedB, "公開中B", readmodel.NPCAvatarStatusPublished, n.DeckPublished1),
		// 公開中でなければ出ないことだけを確かめるので、使うデッキは公開中のものにする
		npcAvatarMetadata(n.Draft, "準備中", readmodel.NPCAvatarStatusDraft, n.DeckPublished1),
		npcAvatarMetadata(n.Retired, "使用不可", readmodel.NPCAvatarStatusRetired, n.DeckPublished1),
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
	return n, nil
}

func npcDeckMetadata(id uuid.UUID, name string, status readmodel.NPCDeckStatus, cards []readmodel.DeckCard) readmodel.NPCDeckMetadata {
	return readmodel.NPCDeckMetadata{ID: id, Name: name, Status: status, Cards: cards}
}

func npcAvatarMetadata(id uuid.UUID, name string, status readmodel.NPCAvatarStatus, decks ...uuid.UUID) readmodel.NPCAvatarMetadata {
	return readmodel.NPCAvatarMetadata{ID: id, Name: name, Status: status, Decks: decks}
}

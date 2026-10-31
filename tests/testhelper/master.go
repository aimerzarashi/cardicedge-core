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

// 見本のマスタの公開中のスターターデッキ。デッキ 1 は A が 4 枚・B が 1 枚、デッキ 2 は A が 2 枚・B が 3 枚・C が 2 枚
// （所持カードは、カードごとに多いほうの枚数になる: A が 4 枚・B が 3 枚・C が 2 枚）。
// 準備中・使用不可のスターターデッキも写してあり（C を 5 枚持つので、配られれば所持カードが変わる）、配られないことを確かめる
type Master struct {
	CardA, CardB, CardC uuid.UUID
	// PublishMaster だけが写す
	Boards Boards
}

// 見本のマスタのボード。公開中が 2 つ（名前の順に A・B）、公開中でないものが 2 つ。
// 準備中は make publish-master が写さないが、写されても出ないことを確かめるために入れる（スターターデッキの準備中と同じ）
type Boards struct {
	PublishedA, PublishedB, Draft, Retired uuid.UUID
}

// 公開は、studio の read model の cards・starter_decks・game_rules の行を game の DB に写すことなので（make publish-master）、
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
	return master, err
}

// カード・スターターデッキだけを写す（game_rules を写し忘れた状態を確かめるため）
func PublishMasterWithoutGameRules(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := publishCards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := PublishStarterDecks(ctx, gameDB, master); err != nil {
		return Master{}, err
	}
	return master, publishUnpublishedStarterDecks(ctx, gameDB, master)
}

// 公開中のスターターデッキが 1 つも無い状態で写す（カード・準備中と使用不可のスターターデッキ・game_rules。
// 公開中が無ければ配布が進まないことを確かめるため）。公開中のものは、後から PublishStarterDecks で足せる
func PublishMasterWithoutPublishedStarterDecks(ctx context.Context, gameDB *sql.DB) (Master, error) {
	master, err := publishCards(ctx, gameDB)
	if err != nil {
		return Master{}, err
	}
	if err := publishUnpublishedStarterDecks(ctx, gameDB, master); err != nil {
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

// 公開中のスターターデッキ 2 つを写す
func PublishStarterDecks(ctx context.Context, gameDB *sql.DB, master Master) error {
	fire := readmodel.StarterDeckMetadata{Name: "火", Status: readmodel.StarterDeckStatusPublished, Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 4}, {CardID: master.CardB, Count: 1}}}
	water := readmodel.StarterDeckMetadata{Name: "水", Status: readmodel.StarterDeckStatusPublished, Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 2}, {CardID: master.CardB, Count: 3}, {CardID: master.CardC, Count: 2}}}
	// sort_name は並びを決めるだけの値（読み手は sort_name の順に配る）
	return insertStarterDecks(ctx, gameDB, []starterDeckRow{{"1", fire}, {"2", water}})
}

// C を 5 枚持つので、配られれば所持カードが変わる
func publishUnpublishedStarterDecks(ctx context.Context, gameDB *sql.DB, master Master) error {
	draft := readmodel.StarterDeckMetadata{Name: "準備中", Status: readmodel.StarterDeckStatusDraft, Cards: []readmodel.DeckCard{{CardID: master.CardC, Count: 5}}}
	retired := readmodel.StarterDeckMetadata{Name: "使用不可", Status: readmodel.StarterDeckStatusRetired, Cards: []readmodel.DeckCard{{CardID: master.CardC, Count: 5}}}
	return insertStarterDecks(ctx, gameDB, []starterDeckRow{{"3", draft}, {"4", retired}})
}

type starterDeckRow struct {
	sortName string
	metadata readmodel.StarterDeckMetadata
}

// 1 つのトランザクションで書く（make publish-master と同じ）。行ごとに書くと、登録の後に写すテストで、配布が 1 つ目の
// デッキだけを読んで配ることがある（core#53）
func insertStarterDecks(ctx context.Context, gameDB *sql.DB, decks []starterDeckRow) error {
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
		row := models.StarterDeck{StarterDeckID: deck.metadata.ID.String(), SortName: deck.sortName, Status: string(deck.metadata.Status), Metadata: metadata}
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

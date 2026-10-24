package testhelper

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 見本のマスタのスターターデッキセット。デッキ 1 は A が 4 枚・B が 1 枚、デッキ 2 は A が 2 枚・B が 3 枚・C が 2 枚
// （所持カードは、カードごとに多いほうの枚数になる: A が 4 枚・B が 3 枚・C が 2 枚）
type Master struct {
	CardA, CardB, CardC uuid.UUID
}

// 公開は、studio の read model の cards・starter_deck_sets の行を game の DB に写すことなので（make publish-master）、
// 行を直接入れる。metadata は書き手（studio の read model）の型で作り、読み手と同じ形であることも確かめる。
// 行を入れるのは、公開されていない状態から公開された状態への変わり目を確かめる（登録が先、公開が後）ため、アプリの起動とは別にしてある
func PublishMaster(ctx context.Context, gameDB *sql.DB) (Master, error) {
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
	metadata, err := json.Marshal(readmodel.StarterDeckSetMetadata{
		ID: uuid.New(),
		Decks: []readmodel.StarterDeckSetDeck{
			{Name: "火", Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 4}, {CardID: master.CardB, Count: 1}}},
			{Name: "水", Cards: []readmodel.DeckCard{{CardID: master.CardA, Count: 2}, {CardID: master.CardB, Count: 3}, {CardID: master.CardC, Count: 2}}},
		},
	})
	if err != nil {
		return Master{}, err
	}
	set := models.StarterDeckSet{StarterDeckSetID: uuid.NewString(), Metadata: metadata}
	return master, set.Insert(ctx, gameDB, boil.Infer())
}

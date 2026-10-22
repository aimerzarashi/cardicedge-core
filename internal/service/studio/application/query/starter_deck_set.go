package query

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// 未設定なら Decks は空・Revision は 0・UpdatedAt は nil（エラーにしない。画面は同じ形で表示し、そのまま設定に進める。core#25）
type StarterDeckSet struct {
	Decks     []StarterDeck
	Revision  int64
	UpdatedAt *time.Time
}

// カード名・種類は持たない（presentation 層が CardHandler.Get で引く）
type StarterDeck struct {
	Name   string
	Cards  []PresetDeckCard
	Source StarterDeckSource
}

type StarterDeckSource struct {
	PresetDeckID uuid.UUID
	// 写したときのプリセットデッキの revision
	Revision int64
	// 写した後に、元のプリセットデッキが変わっている（反映し忘れに気づけるように）。元が消されていれば false（比べる相手が無い）
	ChangedSinceCopied bool
	// 元のプリセットデッキが消されている（消されたことを「変わっていない」と見分けられるように。core#32）
	PresetDeckDeleted bool
}

// 未設定なら found は false
type GetStarterDeckSetFunc func(ctx context.Context) (set StarterDeckSet, found bool, err error)

// 公開 ID で引く今の revision。無い（削除した）プリセットデッキは結果に含めない
type GetPresetDeckRevisionsFunc func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error)

type StarterDeckSetHandler struct {
	getStarterDeckSet      GetStarterDeckSetFunc
	getPresetDeckRevisions GetPresetDeckRevisionsFunc
}

func NewStarterDeckSetHandler(getStarterDeckSet GetStarterDeckSetFunc, getPresetDeckRevisions GetPresetDeckRevisionsFunc) *StarterDeckSetHandler {
	return &StarterDeckSetHandler{getStarterDeckSet: getStarterDeckSet, getPresetDeckRevisions: getPresetDeckRevisions}
}

// 「写した後に変わったか」は保存せず、ここで 2 つの read model を突き合わせて出す（どちらか 1 つの集約からは投影できないため。
// 投影の遅れ方が違うので、一瞬だけ古い組み合わせが見えることがあるが、表示のための値なので受け入れる）
func (q *StarterDeckSetHandler) Get(ctx context.Context) (StarterDeckSet, error) {
	set, found, err := q.getStarterDeckSet(ctx)
	if err != nil || !found {
		return StarterDeckSet{Decks: []StarterDeck{}}, err
	}
	ids := make([]uuid.UUID, 0, len(set.Decks))
	for _, deck := range set.Decks {
		ids = append(ids, deck.Source.PresetDeckID)
	}
	revisions, err := q.getPresetDeckRevisions(ctx, ids)
	if err != nil {
		return StarterDeckSet{}, err
	}
	for i, deck := range set.Decks {
		current, ok := revisions[deck.Source.PresetDeckID]
		set.Decks[i].Source.ChangedSinceCopied = ok && current != deck.Source.Revision
		set.Decks[i].Source.PresetDeckDeleted = !ok
	}
	return set, nil
}

package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

// プリセットデッキと配布デッキの、カードの枚数の入力の確かめ方を共有する（デッキに入れるカードの決まりを 2 か所に書き写さないため）

// 変えるカードだけの、その枚数にする入力（変えないカードは含めなくてよい）
type CardCountInput struct {
	CardID uuid.UUID
	Count  int
}

// 入力を確かめた後の、枚数を決めるカードと外すカード（入力の 0）と、入力の並び（誤りの項目名を入力の位置で返すため）。
// 0 は枚数ではなく「デッキから外す」操作なので、DeckQuantity（1 以上）にはせず、入力を読むここで分ける
type cardCountRequest struct {
	set     map[card.Ref]card.DeckQuantity
	remove  []card.Ref
	entries []cardCountEntry
}

type cardCountEntry struct {
	ref   card.Ref
	field string
}

// 新しく入れる・枚数を増やすカードが、公開中であることを確かめる。減らす・外す・変えないカードは確かめない
// （使用不可にしたカードを、すでに入っているデッキから外せなくならないように）。
// カードの状態を読むのと保存の間に、ほかの操作がカードを使用不可にする競合は許す。プリセットデッキ・配布デッキはカードの状態を
// 守る集約ではなく、使用不可は新しく入れないための印で、入ってしまっても残せる決まりなので、状態の変更と枚数の変更を
// 同じ版に並べるまとめ役の集約を作ってまで防がない（設計メモの「集約をまたぐ制約の守り方」）
func requireAvailable(ctx context.Context, cardStatuses CardStatusesFunc, current map[card.Ref]card.DeckQuantity, requested cardCountRequest) error {
	var increased []cardCountEntry
	refs := make([]card.Ref, 0, len(requested.entries))
	for _, entry := range requested.entries {
		if requested.set[entry.ref].Int() > current[entry.ref].Int() {
			increased = append(increased, entry)
			refs = append(refs, entry.ref)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	statuses, err := cardStatuses(ctx, refs)
	if err != nil {
		return err
	}
	invalid := input.Errors{}
	for _, entry := range increased {
		if statuses[entry.ref] != card.StatusPublished {
			invalid = append(invalid, &input.Unavailable{Field: entry.field})
		}
	}
	if len(invalid) > 0 {
		return invalid
	}
	return nil
}

// 誤りは最初の 1 つで止めず、まとめて input.Errors で返す。項目名は cards.<番号>.cardId のように一覧の位置を含める。
// 一覧が長すぎるときは、1 件ずつの確かめをせずに返す（存在しない ID を大量に送られても、その数だけ処理しないため）
func parseCardCounts(ctx context.Context, entityRefs domain.EntityRefResolver, rules gamerules.Deck, cards []CardCountInput) (cardCountRequest, error) {
	if len(cards) > rules.TotalCardCount {
		return cardCountRequest{}, input.Errors{&input.OutOfRange{Field: "cards", Min: 0, Max: rules.TotalCardCount}}
	}
	ids := make([]uuid.UUID, 0, len(cards))
	for _, requested := range cards {
		ids = append(ids, requested.CardID)
	}
	refs, err := entityRefs.ResolveAll(ctx, ids)
	if err != nil {
		return cardCountRequest{}, err
	}
	invalid := input.Errors{}
	set := make(map[card.Ref]card.DeckQuantity, len(cards))
	var remove []card.Ref
	entries := make([]cardCountEntry, 0, len(cards))
	seen := make(map[uuid.UUID]bool, len(cards))
	for i, requested := range cards {
		cardField := fmt.Sprintf("cards.%d.cardId", i)
		if seen[requested.CardID] {
			invalid = append(invalid, &input.Duplicated{Field: cardField})
		}
		seen[requested.CardID] = true
		// 行が無い・カードの種類でない、はどちらも、指したカードが無いとして扱う
		entityRef, found := refs[requested.CardID]
		ref, err := card.NewRef(entityRef.EntityType(), entityRef.ID())
		if !found || errors.Is(err, card.ErrNotCard) {
			invalid = append(invalid, &input.NotFound{Field: cardField})
		} else if err != nil {
			return cardCountRequest{}, err
		}
		entries = append(entries, cardCountEntry{ref: ref, field: cardField})
		if requested.Count == 0 {
			remove = append(remove, ref)
			continue
		}
		quantity, err := card.NewDeckQuantity(requested.Count, rules.MaxCardDuplicates)
		if errors.Is(err, card.ErrQuantityNotPositive) || errors.Is(err, card.ErrDeckQuantityOverMax) {
			invalid = append(invalid, &input.OutOfRange{Field: fmt.Sprintf("cards.%d.count", i), Min: 0, Max: rules.MaxCardDuplicates.Int()})
		} else if err != nil {
			return cardCountRequest{}, err
		}
		set[ref] = quantity
	}
	if len(invalid) > 0 {
		return cardCountRequest{}, invalid
	}
	return cardCountRequest{set: set, remove: remove, entries: entries}, nil
}

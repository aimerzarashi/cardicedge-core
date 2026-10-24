// マスタ（game の DB に公開された、運営が決めた配布物の定義）を読む。読み出しの関数は infrastructure/repository の実装を
// internal/service/game が渡す
package master

import (
	"context"

	"github.com/google/uuid"
)

// 配布するスターターデッキセット。studio の read model の型を使わず、配布に要る項目だけを持つ
type StarterDeckSet struct {
	Decks []StarterDeck
}

type StarterDeck struct {
	Name  string
	Cards []DeckCard
}

// カードは公開 ID で指す
type DeckCard struct {
	CardID uuid.UUID
	Count  int
}

// まだ公開されていなければ found は false（エラーにしない。登録の直後に公開が間に合っていないだけで待てば直るかは、呼ぶ側が決める）
type GetStarterDeckSetFunc func(ctx context.Context) (set StarterDeckSet, found bool, err error)

// マスタ（game の DB に公開された、運営が決めた配布物の定義）を読む。読み出しの関数は infrastructure/repository の実装を
// internal/service/game が渡す
package master

import (
	"context"

	"github.com/google/uuid"
)

// 配るスターターデッキ。studio の read model の型を使わず、配布に要る項目だけを持つ
type StarterDeck struct {
	Name  string
	Cards []DeckCard
}

// カードは公開 ID で指す
type DeckCard struct {
	CardID uuid.UUID
	Count  int
}

// 公開中のスターターデッキを、決まった順（名前の正規化 → 公開 ID）で返す。1 つも無ければ空（エラーにしない。
// 公開が間に合っていないだけで待てば直るかは、呼ぶ側が決める）。公開中でないもの（準備中・使用不可）は返さない
type ListPublishedStarterDecksFunc func(ctx context.Context) ([]StarterDeck, error)

// マスタ（game の DB に公開された、運営が決めた配布物の定義）を読む。読み出しの関数は infrastructure/repository の実装を
// internal/service/game が渡す
package master

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
)

// 配る配布デッキ。studio の read model の型を使わず、配布に要る項目だけを持つ
type GrantDeck struct {
	Name  string
	Cards []DeckCard
}

// カードは公開 ID で指す
type DeckCard struct {
	CardID uuid.UUID
	Count  int
}

// 配る中身。公開中のスターターセットが指す配布デッキ（セットが持つ順。公開中でないものは飛ばす）と、解放するボードの内部 ID
// （セットが持つ順。ボードの中身は読まない。make publish-master が studio の内部 ID も写す）
type StarterSet struct {
	GrantDecks   []GrantDeck
	UnlockBoards []board.ID
	// 解放する NPC のアバターの game の内部 ID（セットが持つ順）
	UnlockNPCAvatars []npcavatar.ID
}

// 公開中のスターターセットの中身を返す。公開中のセットが無ければ found が false（エラーにしない。
// 公開が間に合っていないだけで待てば直るかは、呼ぶ側が決める）。公開中でないもの（準備中・使用不可）は読まない
type GetPublishedStarterSetFunc func(ctx context.Context) (set StarterSet, found bool, err error)

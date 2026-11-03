package starterset

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
)

// domain_reference_types の名前。スターターセットが配るデッキ・解放するボードを、使っている間は消せないようにする。
// 配るデッキも解放するボードも公開中のものだけ指定でき、公開中のものは準備中へ戻らず消せないので、通常この参照で消せなくなることは
// ない。確かめ忘れても DB が止める、最後の備え（npc_avatar.deck と同じ）
var (
	grantDeckReferenceType = domain.NewReferenceType("starter_set.grant_deck")
	boardReferenceType     = domain.NewReferenceType("starter_set.board")
)

func ReferenceTypes() []domain.ReferenceType {
	return []domain.ReferenceType{grantDeckReferenceType, boardReferenceType}
}

func GrantDeckReferenceType() domain.ReferenceType {
	return grantDeckReferenceType
}

func BoardReferenceType() domain.ReferenceType {
	return boardReferenceType
}

// スターターセットが参照する配布デッキを grantDecks にそろえる（npcavatar.UpdateDeckReferencesFunc と同じ形）
type UpdateGrantDeckReferencesFunc func(ctx context.Context, id ID, grantDecks []grantdeck.ID) error

// スターターセットが参照するボードを boards にそろえる
type UpdateBoardReferencesFunc func(ctx context.Context, id ID, boards []board.ID) error

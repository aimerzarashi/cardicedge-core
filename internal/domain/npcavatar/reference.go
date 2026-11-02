package npcavatar

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
)

// domain_reference_types の名前。NPC のアバターが使うデッキを、使っている間は消せないようにする。
// NPC のデッキは準備中のものだけ消せて公開後に準備中へ戻らず、使えるのは公開中のデッキだけなので、通常この参照で消せなくなることはない。
// 確かめ忘れても DB が止める、最後の備え
var deckReferenceType = domain.NewReferenceType("npc_avatar.deck")

func ReferenceTypes() []domain.ReferenceType {
	return []domain.ReferenceType{deckReferenceType}
}

func DeckReferenceType() domain.ReferenceType {
	return deckReferenceType
}

// NPC のアバターが参照するデッキを decks にそろえる（npcdeck.UpdateCardReferencesFunc と同じ形）
type UpdateDeckReferencesFunc func(ctx context.Context, id ID, decks []npcdeck.ID) error

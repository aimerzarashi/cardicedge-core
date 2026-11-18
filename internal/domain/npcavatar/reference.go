package npcavatar

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
)

// domain_reference_types の名前。NPC のアバターが使うデッキを、使っている間は消せないようにする。
// NPC のデッキは準備中のものだけ消せて公開後に準備中へ戻らず、使えるのは公開中のデッキだけなので、通常この参照で消せなくなることはない。
// 確かめ忘れても DB が止める、最後の備え
var deckReferenceType = domain.NewReferenceType("npc_avatar.deck")

// 使う思考パターンも同じ理由で参照にする（思考パターンも準備中のものだけ消せて、公開後に準備中へ戻らない）
var mindsetReferenceType = domain.NewReferenceType("npc_avatar.mindset")

func ReferenceTypes() []domain.ReferenceType {
	return []domain.ReferenceType{deckReferenceType, mindsetReferenceType}
}

func DeckReferenceType() domain.ReferenceType {
	return deckReferenceType
}

func MindsetReferenceType() domain.ReferenceType {
	return mindsetReferenceType
}

// NPC のアバターが参照するデッキを decks にそろえる（npcdeck.UpdateCardReferencesFunc と同じ形）
type UpdateDeckReferencesFunc func(ctx context.Context, id ID, decks []npcdeck.ID) error

// NPC のアバターが参照する思考パターンを mindsets にそろえる（UpdateDeckReferencesFunc と同じ形）
type UpdateMindsetReferencesFunc func(ctx context.Context, id ID, mindsets []npcmindset.ID) error

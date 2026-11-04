package master

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
)

// 対戦を始めるときに写す、ボード・デッキの中身。部屋が選んだ時点のものではなく、始める時点で game に写してあるものを読む。
// 選んだ後に使用不可になったものも読めなければならない（対戦は始めた時点の写しで進むため）ので、公開中かは見ない。
// 写していないもの（行・カードの内部 ID が無い）は、待てば直る失敗として返す

// ボードの写し（大きさ・マス・つながり・対）
type GetMatchBoardFunc func(ctx context.Context, id board.ID) (match.Board, error)

// プレイヤーのデッキのカード。枚数は DB の値のまま持ち、ルールの上限で確かめるのは呼ぶ側
type GetPlayerDeckCardsFunc func(ctx context.Context, id deck.ID) ([]DeckCard, error)

// NPC のデッキのカード（プレイヤーのデッキと同じ）
type GetNPCDeckCardsFunc func(ctx context.Context, id npcdeck.ID) ([]DeckCard, error)

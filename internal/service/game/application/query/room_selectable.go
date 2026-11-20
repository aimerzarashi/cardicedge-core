package query

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
)

// 部屋の席・対戦ルールに選べるもの。画面の選択肢を出す query（Player.avatars・decks・npcAvatars・boards）と同じハンドラーを通して調べ、
// 選択肢に出るものと受け付ける値がずれないようにする（専用の read model は、一覧と選べる条件がずれてきたときに作る）。
// 選んだ時の 1 回だけ確かめ、準備完了の時には確かめ直さない
type RoomSelectable struct {
	avatars    *AvatarHandler
	decks      *DeckHandler
	npcAvatars *NPCAvatarHandler
	unlocks    *UnlockHandler
}

func NewRoomSelectable(avatars *AvatarHandler, decks *DeckHandler, npcAvatars *NPCAvatarHandler, unlocks *UnlockHandler) *RoomSelectable {
	return &RoomSelectable{avatars: avatars, decks: decks, npcAvatars: npcAvatars, unlocks: unlocks}
}

// プレイヤー自身のアバター（Player.avatars に出るもの）
func (s *RoomSelectable) HasAvatar(ctx context.Context, playerID uuid.UUID, avatarID uuid.UUID) (bool, error) {
	avatars, err := s.avatars.ListByPlayer(ctx, playerID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(avatars, func(avatar Avatar) bool {
		return avatar.ID == avatarID
	}), nil
}

// プレイヤー自身のデッキ（Player.decks に出るもの）。デッキの枚数の決まりは、今は配るデッキが満たしたものしか持たないので確かめない
func (s *RoomSelectable) HasDeck(ctx context.Context, playerID uuid.UUID, deckID uuid.UUID) (bool, error) {
	_, err := s.decks.Get(ctx, playerID, deckID)
	if errors.Is(err, ErrDeckNotFound) {
		return false, nil
	}
	return err == nil, err
}

// プレイヤーが解放した公開中の NPC のアバター（Player.npcAvatars に出るもの）
func (s *RoomSelectable) HasNPCAvatar(ctx context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID) (bool, error) {
	_, err := s.npcAvatars.GetUnlocked(ctx, playerID, npcAvatarID)
	if errors.Is(err, ErrNPCAvatarNotFound) {
		return false, nil
	}
	return err == nil, err
}

// NPC のアバターに割り当てた公開中のデッキ（Player.npcAvatars の decks に出るもの）。アバターが選べなければ選べない
func (s *RoomSelectable) HasNPCDeck(ctx context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID, npcDeckID uuid.UUID) (bool, error) {
	avatar, err := s.npcAvatars.GetUnlocked(ctx, playerID, npcAvatarID)
	if errors.Is(err, ErrNPCAvatarNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(avatar.Decks, func(deck NPCDeck) bool {
		return deck.ID == npcDeckID
	}), nil
}

// NPC のアバターに割り当てた公開中の思考パターン（Player.npcAvatars の mindsets に出るもの）。アバターが選べなければ選べない
func (s *RoomSelectable) HasNPCMindset(ctx context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID, npcMindsetID uuid.UUID) (bool, error) {
	avatar, err := s.npcAvatars.GetUnlocked(ctx, playerID, npcAvatarID)
	if errors.Is(err, ErrNPCAvatarNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(avatar.Mindsets, func(mindset NPCMindset) bool {
		return mindset.ID == npcMindsetID
	}), nil
}

// プレイヤーが解放した公開中のボード（Player.boards に出るもの）
func (s *RoomSelectable) HasBoard(ctx context.Context, playerID uuid.UUID, boardID uuid.UUID) (bool, error) {
	boards, err := s.unlocks.ListBoardsByPlayer(ctx, playerID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(boards, func(board Board) bool {
		return board.ID == boardID
	}), nil
}

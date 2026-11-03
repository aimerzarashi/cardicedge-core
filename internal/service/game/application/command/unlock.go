package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type UnlockHandler struct {
	eventStore   domain.EventStore[unlock.ID, unlock.Event, unlock.State]
	identifier   domain.Identifier[unlock.ID]
	updatePlayer unlock.UpdatePlayerFunc
	now          func() time.Time
}

func NewUnlockHandler(
	eventStore domain.EventStore[unlock.ID, unlock.Event, unlock.State],
	identifier domain.Identifier[unlock.ID],
	updatePlayer unlock.UpdatePlayerFunc,
	now func() time.Time,
) *UnlockHandler {
	return &UnlockHandler{eventStore: eventStore, identifier: identifier, updatePlayer: updatePlayer, now: now}
}

// プレイヤーの解放を作る（何も解放していない）。そのプレイヤーの解放がもうあれば（配布のやり直し）、何もしない
func (c *UnlockHandler) Create(ctx context.Context, playerID player.ID) error {
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, _, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.updatePlayer(ctx, id, playerID); err != nil {
			return struct{}{}, err
		}
		events, err := unlock.Decide(unlock.InitialState(), &unlock.CreateCommand{ID: id, Player: playerID}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, unlock.ErrAlreadyCreated) {
		return nil
	}
	return err
}

// ボードを解放する。すでに解放しているものは飛ばし、新しいものが無ければ何も起きない（false）。
// 解放の集約がまだ作られていなければ unlock.ErrInvalidState
func (c *UnlockHandler) UnlockBoards(ctx context.Context, unlockID unlock.ID, boards []board.ID) (unlocked bool, err error) {
	return rdb.Transact(ctx, func(ctx context.Context) (bool, error) {
		state, err := c.eventStore.Load(ctx, unlockID)
		if err != nil {
			return false, err
		}
		events, err := unlock.Decide(state, &unlock.UnlockBoardsCommand{Boards: boards}, c.now())
		if err != nil {
			return false, err
		}
		return len(events) > 0, c.eventStore.Save(ctx, events)
	})
}

// 解放の持ち主。配布の流れが、unlock のイベントから自分の配布を引くのに使う
func (c *UnlockHandler) Player(ctx context.Context, unlockID unlock.ID) (player.ID, error) {
	state, err := c.eventStore.Load(ctx, unlockID)
	if err != nil {
		return 0, err
	}
	current, ok := state.(unlock.Unlock)
	if !ok {
		return 0, unlock.ErrInvalidState
	}
	return current.Player, nil
}

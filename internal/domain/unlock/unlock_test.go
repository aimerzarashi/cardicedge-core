package unlock_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock/unlocktest"
)

var (
	createdAt  = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	unlockedAt = time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
)

func boardID(t *testing.T, value int64) board.ID {
	t.Helper()
	return boardtest.ID(t, value)
}

func createCommand(t *testing.T) *unlock.CreateCommand {
	t.Helper()
	return &unlock.CreateCommand{ID: unlocktest.ID(t, 30), Player: playertest.ID(t, 10)}
}

func create(t *testing.T) (unlock.State, unlock.Event) {
	t.Helper()
	events, err := unlock.Decide(unlock.InitialState(), createCommand(t), createdAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := unlock.Evolve(unlock.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func apply(t *testing.T, state unlock.State, cmd unlock.Command) (unlock.State, []unlock.Event) {
	t.Helper()
	events, err := unlock.Decide(state, cmd, unlockedAt)
	require.NoError(t, err)
	for _, event := range events {
		state, err = unlock.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func TestDecide_CreateEmitsCreatedEventWithPlayerOnly(t *testing.T) {
	cmd := createCommand(t)

	_, event := create(t)

	require.Equal(t, &unlock.CreatedEvent{
		EventMeta: unlock.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: createdAt},
		Player:    cmd.Player,
	}, event)
}

func TestDecide_RejectsCreateWhenAlreadyCreated(t *testing.T) {
	created, _ := create(t)

	_, err := unlock.Decide(created, createCommand(t), createdAt)

	require.ErrorIs(t, err, unlock.ErrAlreadyCreated)
}

func TestEvolve_CreatedEventMakesUnlockWithoutBoards(t *testing.T) {
	cmd := createCommand(t)

	state, _ := create(t)

	require.Equal(t, unlock.Unlock{ID: cmd.ID, Version: 1, Player: cmd.Player}, state)
}

func TestDecide_UnlockEmitsOnlyNewBoardsInGivenOrder(t *testing.T) {
	created, _ := create(t)
	first, _ := apply(t, created, &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 2), boardID(t, 1)}})

	state, events := apply(t, first, &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 1), boardID(t, 3), boardID(t, 3), boardID(t, 4)}})

	require.Equal(t, []unlock.Event{&unlock.BoardsUnlockedEvent{
		EventMeta: unlock.EventMeta{EntityID: unlocktest.ID(t, 30), AggregateVersion: 3, OccurredAt: unlockedAt},
		Boards:    []board.ID{boardID(t, 3), boardID(t, 4)},
	}}, events)
	require.Equal(t, []board.ID{boardID(t, 2), boardID(t, 1), boardID(t, 3), boardID(t, 4)}, state.(unlock.Unlock).Boards)
	require.Equal(t, int64(3), state.(unlock.Unlock).Version)
}

func TestDecide_UnlockEmitsNothingWhenNothingIsNew(t *testing.T) {
	created, _ := create(t)
	unlocked, _ := apply(t, created, &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 1)}})
	tests := []struct {
		name   string
		state  unlock.State
		boards []board.ID
	}{
		{name: "nothing_requested", state: created, boards: nil},
		{name: "already_unlocked", state: unlocked, boards: []board.ID{boardID(t, 1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := unlock.Decide(tt.state, &unlock.UnlockCommand{Boards: tt.boards}, unlockedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func npcAvatarID(t *testing.T, value int64) npcavatar.ID {
	t.Helper()
	return npcavatartest.ID(t, value)
}

func TestDecide_UnlockEmitsOnlyNewNPCAvatarsInGivenOrderAndSkipsUnlocked(t *testing.T) {
	created, _ := create(t)
	first, _ := apply(t, created, &unlock.UnlockCommand{NPCAvatars: []npcavatar.ID{npcAvatarID(t, 2), npcAvatarID(t, 1)}})

	state, events := apply(t, first, &unlock.UnlockCommand{NPCAvatars: []npcavatar.ID{npcAvatarID(t, 1), npcAvatarID(t, 3), npcAvatarID(t, 3)}})
	_, none := apply(t, state, &unlock.UnlockCommand{NPCAvatars: []npcavatar.ID{npcAvatarID(t, 3)}})

	require.Equal(t, []unlock.Event{&unlock.NPCAvatarsUnlockedEvent{
		EventMeta:  unlock.EventMeta{EntityID: unlocktest.ID(t, 30), AggregateVersion: 3, OccurredAt: unlockedAt},
		NPCAvatars: []npcavatar.ID{npcAvatarID(t, 3)},
	}}, events)
	require.Equal(t, []npcavatar.ID{npcAvatarID(t, 2), npcAvatarID(t, 1), npcAvatarID(t, 3)}, state.(unlock.Unlock).NPCAvatars)
	require.Empty(t, none)
}

// コマンドは種類をまとめ、イベントは種類ごとに分ける（版は出したイベントの数だけ進む）。新しいものが無い種類のイベントは出ない
func TestDecide_UnlockEmitsOneEventPerKindInOneCommand(t *testing.T) {
	created, _ := create(t)

	state, events := apply(t, created, &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 1)}, NPCAvatars: []npcavatar.ID{npcAvatarID(t, 5)}})

	require.Equal(t, []unlock.Event{
		&unlock.BoardsUnlockedEvent{EventMeta: unlock.EventMeta{EntityID: unlocktest.ID(t, 30), AggregateVersion: 2, OccurredAt: unlockedAt}, Boards: []board.ID{boardID(t, 1)}},
		&unlock.NPCAvatarsUnlockedEvent{EventMeta: unlock.EventMeta{EntityID: unlocktest.ID(t, 30), AggregateVersion: 3, OccurredAt: unlockedAt}, NPCAvatars: []npcavatar.ID{npcAvatarID(t, 5)}},
	}, events)
	require.Equal(t, int64(3), state.(unlock.Unlock).Version)
	_, onlyNPC := apply(t, state, &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 1)}, NPCAvatars: []npcavatar.ID{npcAvatarID(t, 6)}})
	require.Len(t, onlyNPC, 1)
	require.IsType(t, &unlock.NPCAvatarsUnlockedEvent{}, onlyNPC[0])
}

func TestEvolve_RejectsNPCAvatarsUnlockedEventBeforeCreated(t *testing.T) {
	_, err := unlock.Evolve(unlock.InitialState(), &unlock.NPCAvatarsUnlockedEvent{NPCAvatars: []npcavatar.ID{npcAvatarID(t, 1)}})

	require.ErrorIs(t, err, unlock.ErrInvalidState)
}

func TestDecide_RejectsUnlockBeforeCreated(t *testing.T) {
	_, err := unlock.Decide(unlock.InitialState(), &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 1)}}, unlockedAt)

	require.ErrorIs(t, err, unlock.ErrInvalidState)
}

func TestEvolve_DoesNotChangeGivenState(t *testing.T) {
	created, _ := create(t)

	next, _ := apply(t, created, &unlock.UnlockCommand{Boards: []board.ID{boardID(t, 1)}})

	require.Empty(t, created.(unlock.Unlock).Boards)
	require.Len(t, next.(unlock.Unlock).Boards, 1)
}

func TestEvolve_RejectsCreatedEventWhenAlreadyCreated(t *testing.T) {
	created, event := create(t)

	_, err := unlock.Evolve(created, event)

	require.ErrorIs(t, err, unlock.ErrInvalidState)
}

func TestEvolve_RejectsBoardsUnlockedEventBeforeCreated(t *testing.T) {
	_, err := unlock.Evolve(unlock.InitialState(), &unlock.BoardsUnlockedEvent{Boards: []board.ID{boardID(t, 1)}})

	require.ErrorIs(t, err, unlock.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	_, created := create(t)
	meta := unlock.EventMeta{EntityID: unlocktest.ID(t, 30), AggregateVersion: 2, OccurredAt: unlockedAt}
	events := []unlock.Event{
		created,
		&unlock.BoardsUnlockedEvent{EventMeta: meta, Boards: []board.ID{boardID(t, 0xb1), boardID(t, 0xb2)}},
		&unlock.NPCAvatarsUnlockedEvent{EventMeta: meta, NPCAvatars: []npcavatar.ID{npcAvatarID(t, 60), npcAvatarID(t, 61)}},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := unlock.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := unlock.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(unlock.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestPlayerKey_DiffersPerPlayer(t *testing.T) {
	require.NotEqual(t, unlock.PlayerKey(playertest.ID(t, 10)), unlock.PlayerKey(playertest.ID(t, 11)))
}

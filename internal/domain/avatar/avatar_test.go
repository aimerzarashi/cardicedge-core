package avatar_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
)

var (
	registeredAt = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

func registerCommand(t *testing.T) *avatar.RegisterCommand {
	t.Helper()
	return &avatar.RegisterCommand{
		ID:       avatartest.ID(t, 30),
		PublicID: publicID,
		Player:   playertest.ID(t, 10),
		Name:     avatartest.Name(t, "アリス"),
	}
}

func register(t *testing.T) (avatar.State, avatar.Event) {
	t.Helper()
	events, err := avatar.Decide(avatar.InitialState(), registerCommand(t), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := avatar.Evolve(avatar.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	cmd := registerCommand(t)

	_, event := register(t)

	require.Equal(t, &avatar.RegisteredEvent{
		EventMeta: avatar.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  cmd.PublicID,
		Player:    cmd.Player,
		Name:      cmd.Name,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	registered, _ := register(t)

	_, err := avatar.Decide(registered, registerCommand(t), registeredAt)

	require.ErrorIs(t, err, avatar.ErrAlreadyRegistered)
}

func TestEvolve_RegisteredEventMakesAvatar(t *testing.T) {
	cmd := registerCommand(t)

	state, _ := register(t)

	require.Equal(t, avatar.Avatar{
		ID: cmd.ID, Version: 1, PublicID: cmd.PublicID,
	}, state)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	registered, event := register(t)

	_, err := avatar.Evolve(registered, event)

	require.ErrorIs(t, err, avatar.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	_, event := register(t)
	require.Equal(t, 1, event.SchemaVersion())
	golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
	require.NoError(t, err)

	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(golden), string(payload))

	restored, err := avatar.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := avatar.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(avatar.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestNewName_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{name: "empty", value: "", want: avatar.ErrNameEmpty},
		{name: "blank", value: " 　 ", want: avatar.ErrNameEmpty},
		{name: "too_long", value: strings.Repeat("あ", avatar.MaxNameLength+1), want: avatar.ErrNameTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := avatar.NewName(tt.value)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestNewName_AcceptsMaxLengthAfterTrimming(t *testing.T) {
	name, err := avatar.NewName(" " + strings.Repeat("あ", avatar.MaxNameLength) + " ")

	require.NoError(t, err)
	require.Equal(t, strings.Repeat("あ", avatar.MaxNameLength), name.String())
}

func TestNameKey_UsesNormalizedNameOfPlayer(t *testing.T) {
	full := avatar.NameKey(playertest.ID(t, 10), avatartest.Name(t, "ＡＢＣ"))
	half := avatar.NameKey(playertest.ID(t, 10), avatartest.Name(t, " abc "))

	require.Equal(t, full, half)
	require.Equal(t, avatar.NameKeyType, full.Type())
}

func TestNameKey_DiffersPerPlayer(t *testing.T) {
	require.NotEqual(t,
		avatar.NameKey(playertest.ID(t, 1), avatartest.Name(t, "abc")),
		avatar.NameKey(playertest.ID(t, 2), avatartest.Name(t, "abc")),
	)
}

// 持ち主の ID と名前の数字がつながっても、別の組と同じ key にならない
func TestNameKey_DistinguishesHowPlayerAndNameAreSplit(t *testing.T) {
	require.NotEqual(t,
		avatar.NameKey(playertest.ID(t, 1), avatartest.Name(t, "23")),
		avatar.NameKey(playertest.ID(t, 12), avatartest.Name(t, "3")),
	)
}

func TestPlayerQuota_CountsPerPlayer(t *testing.T) {
	quota := avatar.PlayerQuota(playertest.ID(t, 10))

	require.Equal(t, avatar.PlayerQuotaType, quota.Type())
	require.Equal(t, int64(10), quota.OwnerID())
}

package item_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item/itemtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id           item.ID
	name         card.Name
	registeredBy operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:           itemtest.ID(t, 20),
		name:         cardtest.Name(t, "ポーション"),
		registeredBy: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *item.RegisterCommand {
	return &item.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy}
}

func (f fixture) register(t *testing.T) (item.State, item.Event) {
	t.Helper()
	events, err := item.Decide(item.InitialState(), f.registerCommand(), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := item.Evolve(item.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.register(t)

	require.Equal(t, &item.RegisteredEvent{
		EventMeta:    item.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.registeredBy,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := item.Decide(registered, f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, item.ErrInvalidState)
}

func TestEvolve_RegisteredEventMakesItem(t *testing.T) {
	f := newFixture(t)

	registered, _ := f.register(t)

	require.Equal(t, item.Item{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy, RegisteredAt: registeredAt,
	}, registered)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, event := f.register(t)

	_, err := item.Evolve(registered, event)

	require.ErrorIs(t, err, item.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	_, event := f.register(t)
	require.Equal(t, 1, event.SchemaVersion())
	golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
	require.NoError(t, err)

	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(golden), string(payload))

	restored, err := item.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := item.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(item.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

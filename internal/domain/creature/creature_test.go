package creature_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id           creature.ID
	name         card.Name
	registeredBy operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:           creaturetest.ID(t, 20),
		name:         cardtest.Name(t, "ドラゴン"),
		registeredBy: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *creature.RegisterCommand {
	return &creature.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy}
}

func (f fixture) register(t *testing.T) (creature.State, creature.Event) {
	t.Helper()
	events, err := creature.Decide(creature.InitialState(), f.registerCommand(), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := creature.Evolve(creature.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.register(t)

	require.Equal(t, &creature.RegisteredEvent{
		EventMeta:    creature.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.registeredBy,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := creature.Decide(registered, f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, creature.ErrInvalidState)
}

func TestEvolve_RegisteredEventMakesCreature(t *testing.T) {
	f := newFixture(t)

	registered, _ := f.register(t)

	require.Equal(t, creature.Creature{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy, RegisteredAt: registeredAt,
	}, registered)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, event := f.register(t)

	_, err := creature.Evolve(registered, event)

	require.ErrorIs(t, err, creature.ErrInvalidState)
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

	restored, err := creature.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := creature.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(creature.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

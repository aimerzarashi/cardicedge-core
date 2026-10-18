package operator_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	linkedAt     = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id      operator.ID
	email   operator.Email
	issuer  operator.Issuer
	subject operator.Subject
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:    operatortest.ID(t, 10),
		email: operatortest.Email(t, "operator@example.com"),
		// 古い発行元の名前（cms。core#21 で studio に変えた）のままなのは、golden（testdata/events）と同じ値で比べるため。
		// golden は保存済みのイベントの形の記録で、値を変えても意味は変わらない
		issuer:  operatortest.Issuer(t, "diecardomain-cms"),
		subject: operatortest.Subject(t, "user-1"),
	}
}

func (f fixture) registerCommand() *operator.RegisterCommand {
	return &operator.RegisterCommand{ID: f.id, PublicID: publicID, Email: f.email}
}

func (f fixture) linkCommand() *operator.LinkCommand {
	return &operator.LinkCommand{Issuer: f.issuer, Subject: f.subject}
}

// 決めたイベントを順に適用した状態と、最後のイベントを返す
func (f fixture) apply(t *testing.T, cmds ...operator.Command) (operator.State, operator.Event) {
	t.Helper()
	state := operator.InitialState()
	var last operator.Event
	for i, cmd := range cmds {
		events, err := operator.Decide(state, cmd, []time.Time{registeredAt, linkedAt}[i])
		require.NoError(t, err)
		require.Len(t, events, 1)
		last = events[0]
		state, err = operator.Evolve(state, last)
		require.NoError(t, err)
	}
	return state, last
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.apply(t, f.registerCommand())

	require.Equal(t, &operator.RegisteredEvent{
		EventMeta: operator.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID,
		Email:     f.email,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.apply(t, f.registerCommand())

	_, err := operator.Decide(registered, f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, operator.ErrInvalidState)
}

func TestDecide_LinkEmitsLinkedEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.apply(t, f.registerCommand(), f.linkCommand())

	require.Equal(t, &operator.LinkedEvent{
		EventMeta: operator.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: linkedAt},
		Issuer:    f.issuer,
		Subject:   f.subject,
	}, event)
}

func TestDecide_RejectsLinkBeforeRegister(t *testing.T) {
	_, err := operator.Decide(operator.InitialState(), newFixture(t).linkCommand(), linkedAt)

	require.ErrorIs(t, err, operator.ErrInvalidState)
}

func TestDecide_RejectsLinkWhenAlreadyLinked(t *testing.T) {
	f := newFixture(t)
	linked, _ := f.apply(t, f.registerCommand(), f.linkCommand())
	other := &operator.LinkCommand{Issuer: f.issuer, Subject: operatortest.Subject(t, "user-2")}

	_, err := operator.Decide(linked, other, linkedAt)

	require.ErrorIs(t, err, operator.ErrAlreadyLinked)
}

func TestEvolve_RegisteredAndLinkedEventsMakeLinkedOperator(t *testing.T) {
	f := newFixture(t)

	registered, _ := f.apply(t, f.registerCommand())
	linked, _ := f.apply(t, f.registerCommand(), f.linkCommand())

	require.Equal(t, operator.Operator{
		ID: f.id, Version: 1, PublicID: publicID, Email: f.email, RegisteredAt: registeredAt,
	}, registered)
	require.False(t, registered.(operator.Operator).Linked())
	require.Equal(t, operator.Operator{
		ID: f.id, Version: 2, PublicID: publicID, Email: f.email, RegisteredAt: registeredAt,
		Issuer: f.issuer, Subject: f.subject, LinkedAt: linkedAt,
	}, linked)
	require.True(t, linked.(operator.Operator).Linked())
}

func TestEvolve_RejectsEventInWrongState(t *testing.T) {
	f := newFixture(t)
	registered, registeredEvent := f.apply(t, f.registerCommand())
	linked, linkedEvent := f.apply(t, f.registerCommand(), f.linkCommand())
	tests := []struct {
		name  string
		state operator.State
		event operator.Event
	}{
		{name: "registered_twice", state: registered, event: registeredEvent},
		{name: "linked_before_register", state: operator.InitialState(), event: linkedEvent},
		{name: "linked_twice", state: linked, event: linkedEvent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := operator.Evolve(tt.state, tt.event)

			require.ErrorIs(t, err, operator.ErrInvalidState)
		})
	}
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	_, registered := f.apply(t, f.registerCommand())
	_, linked := f.apply(t, f.registerCommand(), f.linkCommand())
	tests := []struct {
		name  string
		event operator.Event
	}{
		{name: "registered_event", event: registered},
		{name: "linked_event", event: linked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.event
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := operator.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := operator.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(operator.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

func TestNewEmail_NormalizesToLowerCase(t *testing.T) {
	email, err := operator.NewEmail(" Operator@Example.COM ")

	require.NoError(t, err)
	require.Equal(t, "operator@example.com", email.String())
}

func TestNewEmail_RejectsInvalidValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "without_at", value: "operator.example.com"},
		{name: "with_display_name", value: "Operator <operator@example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := operator.NewEmail(tt.value)

			require.Error(t, err)
		})
	}
}

// 区切り文字でつなぐと ("ab", "c") と ("a", "bc") が同じ key になり、別の組を同じ (iss, sub) として扱ってしまう
func TestSubjectKey_DistinguishesHowIssuerAndSubjectAreSplit(t *testing.T) {
	left := operator.SubjectKey(operatortest.Issuer(t, "ab"), operatortest.Subject(t, "c"))
	right := operator.SubjectKey(operatortest.Issuer(t, "a"), operatortest.Subject(t, "bc"))

	require.NotEqual(t, left, right)
	require.Equal(t, operator.SubjectKeyType, left.Type())
}

package card_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
)

func TestNewName_TrimsSurroundingWhitespace(t *testing.T) {
	name, err := card.NewName(" \t　ドラゴン \n")

	require.NoError(t, err)
	require.Equal(t, "ドラゴン", name.String())
}

func TestNewName_AcceptsMaxLength(t *testing.T) {
	_, err := card.NewName(strings.Repeat("あ", 50))

	require.NoError(t, err)
}

func TestNewName_RejectsInvalidValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		err   error
	}{
		{name: "empty", value: "", err: card.ErrNameEmpty},
		{name: "blank", value: " 　\t", err: card.ErrNameEmpty},
		{name: "too_long", value: strings.Repeat("あ", 51), err: card.ErrNameTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := card.NewName(tt.value)

			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestNameKey_UsesNormalizedName(t *testing.T) {
	full := card.NameKey(cardtest.Name(t, "ＡＢＣ"))
	half := card.NameKey(cardtest.Name(t, " abc "))

	require.Equal(t, half, full)
	require.Equal(t, "abc", full.Value())
}

func TestNewStatus_ReadsKnownValues(t *testing.T) {
	for _, want := range []card.Status{card.StatusDraft, card.StatusPublished, card.StatusRetired} {
		status, err := card.NewStatus(want.String())

		require.NoError(t, err)
		require.Equal(t, want, status)
	}
}

func TestNewStatus_RejectsUnknownValue(t *testing.T) {
	_, err := card.NewStatus("archived")

	require.ErrorIs(t, err, card.ErrStatusUnknown)
}

func TestStatus_Publish(t *testing.T) {
	tests := []struct {
		name        string
		from        card.Status
		wantChanged bool
	}{
		{name: "draft", from: card.StatusDraft, wantChanged: true},
		{name: "retired", from: card.StatusRetired, wantChanged: true},
		{name: "published", from: card.StatusPublished, wantChanged: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, changed, err := tt.from.Publish()

			require.NoError(t, err)
			require.Equal(t, card.StatusPublished, next)
			require.Equal(t, tt.wantChanged, changed)
		})
	}
}

func TestStatus_Retire(t *testing.T) {
	tests := []struct {
		name        string
		from        card.Status
		wantNext    card.Status
		wantChanged bool
		wantErr     error
	}{
		{name: "published", from: card.StatusPublished, wantNext: card.StatusRetired, wantChanged: true},
		{name: "retired", from: card.StatusRetired, wantNext: card.StatusRetired, wantChanged: false},
		{name: "draft", from: card.StatusDraft, wantNext: card.StatusDraft, wantErr: card.ErrInvalidStatusTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, changed, err := tt.from.Retire()

			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantNext, next)
			require.Equal(t, tt.wantChanged, changed)
		})
	}
}

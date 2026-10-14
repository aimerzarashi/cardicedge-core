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

func TestNormalizeName_UnifiesWidthAndCase(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "full_width", value: "ＡＢＣ１２３", want: "abc123"},
		{name: "upper_case", value: "Apple", want: "apple"},
		{name: "upper_case_made_by_nfkc", value: "Ⅻ", want: "xii"},
		{name: "half_width_katakana", value: "ﾄﾞﾗｺﾞﾝ", want: "ドラゴン"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, card.NormalizeName(tt.value))
		})
	}
}

func TestNameKey_UsesNormalizedName(t *testing.T) {
	full := card.NameKey(cardtest.Name(t, "ＡＢＣ"))
	half := card.NameKey(cardtest.Name(t, " abc "))

	require.Equal(t, half, full)
	require.Equal(t, "abc", full.Value())
}

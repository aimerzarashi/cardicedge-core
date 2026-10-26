package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

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
			require.Equal(t, tt.want, domain.NormalizeName(tt.value))
		})
	}
}

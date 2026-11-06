package card_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
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

func TestNewOwnedQuantity_RejectsNotPositive(t *testing.T) {
	_, zero := card.NewOwnedQuantity(0)
	_, negative := card.NewOwnedQuantity(-1)

	require.ErrorIs(t, zero, card.ErrQuantityNotPositive)
	require.ErrorIs(t, negative, card.ErrQuantityNotPositive)
}

func TestNewDeckQuantity_ChecksAgainstMax(t *testing.T) {
	max := cardtest.MaxDeckQuantity(t, 4)
	tests := []struct {
		name  string
		value int
		err   error
	}{
		{name: "zero", value: 0, err: card.ErrQuantityNotPositive},
		{name: "negative", value: -1, err: card.ErrQuantityNotPositive},
		{name: "at_max", value: 4},
		{name: "over_max", value: 5, err: card.ErrDeckQuantityOverMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quantity, err := card.NewDeckQuantity(tt.value, max)

			require.ErrorIs(t, err, tt.err)
			if tt.err == nil {
				require.Equal(t, tt.value, quantity.Int())
			}
		})
	}
}

func TestNewMaxDeckQuantity_RejectsNotPositive(t *testing.T) {
	_, err := card.NewMaxDeckQuantity(0)

	require.ErrorIs(t, err, card.ErrQuantityNotPositive)
}

func TestDeckQuantity_Add(t *testing.T) {
	tests := []struct {
		name        string
		current     int
		delta       int
		want        int
		wantRemains bool
		wantErr     error
	}{
		{name: "adds_to_existing", current: 2, delta: 1, want: 3, wantRemains: true},
		{name: "adds_to_absent_card", current: 0, delta: 2, want: 2, wantRemains: true},
		{name: "reaches_zero", current: 2, delta: -2},
		{name: "goes_below_zero", current: 1, delta: -2, wantErr: card.ErrQuantityNotPositive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// デッキに無いカードの現在の枚数は、map を引いたゼロ値
			cards := map[string]card.DeckQuantity{}
			if tt.current > 0 {
				cards["dragon"] = cardtest.DeckQuantity(t, tt.current)
			}
			current := cards["dragon"]

			next, remains, err := current.Add(tt.delta)

			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantRemains, remains)
			require.Equal(t, tt.want, next.Int())
		})
	}
}

func TestNewRef_AcceptsOnlyCardKinds(t *testing.T) {
	for _, kind := range []domain.EntityType{card.CreatureEntityType, card.ItemEntityType, card.SpellEntityType} {
		ref, err := card.NewRef(kind, 7)

		require.NoError(t, err)
		require.Equal(t, kind, ref.EntityType)
		require.Equal(t, int64(7), ref.ID)
	}
}

func TestNewRef_RejectsInvalidValue(t *testing.T) {
	tests := []struct {
		name       string
		entityType domain.EntityType
		id         int64
		err        error
	}{
		{name: "not_a_card", entityType: domain.NewEntityType("board"), id: 1, err: card.ErrNotCard},
		{name: "zero_id", entityType: card.CreatureEntityType, id: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := card.NewRef(tt.entityType, tt.id)

			require.Error(t, err)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			}
		})
	}
}

func TestRef_KindsMatchTheAggregates(t *testing.T) {
	require.Equal(t, creature.Types().EntityType, card.CreatureEntityType)
	require.Equal(t, item.Types().EntityType, card.ItemEntityType)
	require.Equal(t, spell.Types().EntityType, card.SpellEntityType)
}

func TestRef_JSONRoundTrip(t *testing.T) {
	ref := cardtest.Ref(t, card.ItemEntityType, 9)

	raw, err := json.Marshal(ref)
	require.NoError(t, err)
	var got card.Ref
	require.NoError(t, json.Unmarshal(raw, &got))

	require.JSONEq(t, `{"entity_type":"item","id":9}`, string(raw))
	require.Equal(t, ref, got)
}

// 保存済みのイベントを読み直すときは確かめない（ルールを変えても読めるように）
func TestRef_UnmarshalDoesNotValidate(t *testing.T) {
	var ref card.Ref

	err := json.Unmarshal([]byte(`{"entity_type":"board","id":1}`), &ref)

	require.NoError(t, err)
	require.Equal(t, int64(1), ref.ID)
}

func TestRef_Compare(t *testing.T) {
	creature1 := cardtest.Ref(t, card.CreatureEntityType, 1)
	creature2 := cardtest.Ref(t, card.CreatureEntityType, 2)
	item1 := cardtest.Ref(t, card.ItemEntityType, 1)

	require.Negative(t, creature1.Compare(creature2))
	require.Positive(t, creature2.Compare(creature1))
	require.Zero(t, creature1.Compare(creature1))
	require.Negative(t, creature2.Compare(item1))
}

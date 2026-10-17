package pagination_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
)

type testCursor struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

func (c testCursor) Validate() error {
	if c.ID == 0 {
		return errors.New("id is required")
	}
	return nil
}

func encode(t *testing.T, cursor testCursor) *string {
	t.Helper()
	encoded, err := pagination.EncodeCursor(cursor)
	require.NoError(t, err)
	return &encoded
}

func raw(value string) *string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(value))
	return &encoded
}

func intPtr(value int) *int {
	return &value
}

func TestNewCondition_DefaultsFirstWithoutCursor(t *testing.T) {
	condition, err := pagination.NewCondition[testCursor](nil, nil)

	require.NoError(t, err)
	require.Equal(t, pagination.DefaultFirst, condition.First)
	require.Nil(t, condition.After)
}

func TestNewCondition_AcceptsFirstWithinRange(t *testing.T) {
	tests := []struct {
		name  string
		first int
	}{
		{name: "min", first: pagination.MinFirst},
		{name: "max", first: pagination.MaxFirst},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := pagination.NewCondition[testCursor](intPtr(tt.first), nil)

			require.NoError(t, err)
			require.Equal(t, tt.first, condition.First)
		})
	}
}

func TestNewCondition_RejectsFirstOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		first int
	}{
		{name: "zero", first: 0},
		{name: "negative", first: -1},
		{name: "over_max", first: pagination.MaxFirst + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pagination.NewCondition[testCursor](intPtr(tt.first), nil)

			require.ErrorIs(t, err, pagination.ErrFirstOutOfRange)
		})
	}
}

func TestNewCondition_DecodesEncodedCursor(t *testing.T) {
	cursor := testCursor{Name: "ドラゴン+/=", ID: 1}

	condition, err := pagination.NewCondition[testCursor](nil, encode(t, cursor))

	require.NoError(t, err)
	require.Equal(t, cursor, *condition.After)
}

func TestNewCondition_RejectsInvalidCursor(t *testing.T) {
	tests := []struct {
		name  string
		after *string
	}{
		{name: "empty", after: new(string)},
		{name: "not_base64", after: func() *string { s := "!!!"; return &s }()},
		{name: "standard_base64_padding", after: func() *string {
			s := base64.StdEncoding.EncodeToString([]byte(`{"name":"a","id":1}`))
			return &s
		}()},
		{name: "not_json", after: raw("not json")},
		{name: "unknown_field", after: raw(`{"name":"a","id":1,"extra":true}`)},
		{name: "missing_value", after: raw(`{}`)},
		{name: "trailing_data", after: raw(`{"name":"a","id":1}{}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pagination.NewCondition[testCursor](nil, tt.after)

			require.ErrorIs(t, err, pagination.ErrInvalidCursor)
		})
	}
}

func TestNewCondition_ReportsBothFirstAndCursorErrors(t *testing.T) {
	_, err := pagination.NewCondition[testCursor](intPtr(0), raw(`{}`))

	require.ErrorIs(t, err, pagination.ErrFirstOutOfRange)
	require.ErrorIs(t, err, pagination.ErrInvalidCursor)
}

func TestCondition_FetchLimitIsOneMoreThanFirst(t *testing.T) {
	condition, err := pagination.NewCondition[testCursor](intPtr(10), nil)
	require.NoError(t, err)

	require.Equal(t, 11, condition.FetchLimit())
}

func cursorOf(id int) testCursor {
	return testCursor{ID: id}
}

func TestBuildConnection_ReturnsPageAndNextPageWhenMoreRowsThanFirst(t *testing.T) {
	condition, err := pagination.NewCondition[testCursor](intPtr(2), nil)
	require.NoError(t, err)

	connection, err := pagination.BuildConnection([]int{1, 2, 3}, condition, cursorOf)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, 1, connection.Edges[0].Node)
	require.Equal(t, 2, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
	require.Equal(t, *encode(t, cursorOf(1)), *connection.PageInfo.StartCursor)
	require.Equal(t, *encode(t, cursorOf(2)), *connection.PageInfo.EndCursor)
	require.Equal(t, *connection.PageInfo.EndCursor, connection.Edges[1].Cursor)
}

func TestBuildConnection_HasNoNextPageWhenRowsFitInFirst(t *testing.T) {
	condition, err := pagination.NewCondition[testCursor](intPtr(2), nil)
	require.NoError(t, err)

	connection, err := pagination.BuildConnection([]int{1, 2}, condition, cursorOf)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.False(t, connection.PageInfo.HasNextPage)
}

func TestBuildConnection_ReturnsEmptyEdgesWithoutCursorsWhenNoRows(t *testing.T) {
	condition, err := pagination.NewCondition[testCursor](nil, nil)
	require.NoError(t, err)

	connection, err := pagination.BuildConnection([]int{}, condition, cursorOf)

	require.NoError(t, err)
	require.NotNil(t, connection.Edges)
	require.Empty(t, connection.Edges)
	require.Nil(t, connection.PageInfo.StartCursor)
	require.Nil(t, connection.PageInfo.EndCursor)
}

func TestBuildConnection_HasPreviousPageOnlyWithAfter(t *testing.T) {
	tests := []struct {
		name  string
		after *string
		want  bool
	}{
		{name: "first_page", after: nil, want: false},
		{name: "after_cursor", after: raw(`{"name":"a","id":1}`), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := pagination.NewCondition[testCursor](nil, tt.after)
			require.NoError(t, err)

			connection, err := pagination.BuildConnection([]int{2}, condition, cursorOf)

			require.NoError(t, err)
			require.Equal(t, tt.want, connection.PageInfo.HasPreviousPage)
		})
	}
}

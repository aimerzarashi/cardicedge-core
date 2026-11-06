package query_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 決めた行を返し、渡された条件を覚える読み出しの関数
type fakeRoomReader struct {
	rows      []query.RoomListRow
	playerID  uuid.UUID
	condition query.ListRoomsCondition
	called    bool
	room      query.Room
	err       error
	// 見ている人にできる操作（詳細に付く）
	capabilities    query.RoomCapabilities
	capabilitiesErr error
}

func (r *fakeRoomReader) getCapabilities(context.Context, uuid.UUID, uuid.UUID) (query.RoomCapabilities, error) {
	return r.capabilities, r.capabilitiesErr
}

func (r *fakeRoomReader) list(_ context.Context, playerID uuid.UUID, condition query.ListRoomsCondition) ([]query.RoomListRow, error) {
	r.called = true
	r.playerID = playerID
	r.condition = condition
	return r.rows, nil
}

func (r *fakeRoomReader) get(context.Context, uuid.UUID, uuid.UUID) (query.Room, error) {
	return r.room, r.err
}

func roomRow(at time.Time) query.RoomListRow {
	room := query.Room{ID: uuid.New()}
	cursor := query.RoomListCursor{RegisteredAt: at, RoomID: room.ID}
	return query.RoomListRow{Room: room, Cursor: cursor}
}

func TestRoomHandler_ListReadsOneMoreThanFirstForTheGivenPlayer(t *testing.T) {
	reader := &fakeRoomReader{}
	playerID := uuid.New()

	_, err := query.NewRoomHandler(reader.list, reader.get, reader.getCapabilities, nil).ListByPlayer(t.Context(), playerID, intPtr(10), nil)

	require.NoError(t, err)
	require.Equal(t, playerID, reader.playerID)
	require.Equal(t, query.ListRoomsCondition{Limit: 11}, reader.condition)
}

func TestRoomHandler_ListRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		first *int
		after *string
		want  []input.Error
	}{
		{name: "first_out_of_range", first: intPtr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: stringPtr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeRoomReader{}

			_, err := query.NewRoomHandler(reader.list, reader.get, reader.getCapabilities, nil).ListByPlayer(t.Context(), uuid.New(), tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestRoomHandler_ListPagesWithCursorOfLastRoomAndContinuesFromIt(t *testing.T) {
	base := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	rows := []query.RoomListRow{roomRow(base.Add(2 * time.Second)), roomRow(base.Add(time.Second)), roomRow(base)}
	reader := &fakeRoomReader{rows: rows}
	handler := query.NewRoomHandler(reader.list, reader.get, reader.getCapabilities, nil)

	connection, err := handler.ListByPlayer(t.Context(), uuid.New(), intPtr(2), nil)
	require.NoError(t, err)
	_, err = handler.ListByPlayer(t.Context(), uuid.New(), intPtr(2), connection.PageInfo.EndCursor)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].Room.ID, connection.Edges[0].Node.ID)
	require.Equal(t, rows[1].Room.ID, connection.Edges[1].Node.ID)
	require.True(t, connection.PageInfo.HasNextPage)
	require.Equal(t, &rows[1].Cursor, reader.condition.After)
}

func TestRoomHandler_ListReturnsEmptyEdgesWhenThereIsNoRoom(t *testing.T) {
	reader := &fakeRoomReader{}

	connection, err := query.NewRoomHandler(reader.list, reader.get, reader.getCapabilities, nil).ListByPlayer(t.Context(), uuid.New(), nil, nil)

	require.NoError(t, err)
	require.NotNil(t, connection.Edges)
	require.Empty(t, connection.Edges)
	require.False(t, connection.PageInfo.HasNextPage)
}

func TestRoomHandler_GetReturnsRoomOrNotFound(t *testing.T) {
	room := query.Room{ID: uuid.New(), Revision: 3}
	capabilities := query.RoomCapabilities{Publish: true, ChangeSeatChoice: []bool{true}}
	found := &fakeRoomReader{room: room, capabilities: capabilities}
	missing := &fakeRoomReader{err: query.ErrRoomNotFound}

	got, err := query.NewRoomHandler(found.list, found.get, found.getCapabilities, nil).Get(t.Context(), uuid.New(), uuid.New())
	_, notFound := query.NewRoomHandler(missing.list, missing.get, missing.getCapabilities, nil).Get(t.Context(), uuid.New(), uuid.New())

	require.NoError(t, err)
	want := found.room
	want.Capabilities = &found.capabilities
	require.Equal(t, want, got)
	require.ErrorIs(t, notFound, query.ErrRoomNotFound)
}

func TestRoomHandler_GetDoesNotAskCapabilitiesOfRoomThatIsNotFoundAndListHasNone(t *testing.T) {
	missing := &fakeRoomReader{err: query.ErrRoomNotFound, capabilitiesErr: errors.New("must not be called")}
	listed := &fakeRoomReader{rows: []query.RoomListRow{roomRow(time.Now())}}

	_, notFound := query.NewRoomHandler(missing.list, missing.get, missing.getCapabilities, nil).Get(t.Context(), uuid.New(), uuid.New())
	connection, err := query.NewRoomHandler(listed.list, listed.get, listed.getCapabilities, nil).ListByPlayer(t.Context(), uuid.New(), nil, nil)

	require.ErrorIs(t, notFound, query.ErrRoomNotFound)
	require.NoError(t, err)
	require.Nil(t, connection.Edges[0].Node.Capabilities)
}

func TestRoomListCursor_RequiresTimeAndID(t *testing.T) {
	require.Error(t, query.RoomListCursor{RoomID: uuid.New()}.Validate())
	require.Error(t, query.RoomListCursor{RegisteredAt: time.Now()}.Validate())
	require.NoError(t, query.RoomListCursor{RegisteredAt: time.Now(), RoomID: uuid.New()}.Validate())
}

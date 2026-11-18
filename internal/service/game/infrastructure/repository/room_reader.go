package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// core_game の rooms（部屋の read model）を読む。席のアバター・デッキ・ボードの名前は rooms に書き写さず、読むときに avatars・decks・
// npc_avatars・npc_decks・npc_mindsets・boards から引く（名前が変わっても、部屋の表示に反映されるため）。
// 部屋に入れるのは今は作った人だけなので、持ち主で絞る（ほかの人が参加できるようになるときに、入っている人で絞る）
type RoomReader struct{}

func NewRoomReader() *RoomReader {
	return &RoomReader{}
}

const roomStatusClosed = "closed"

// 並びは索引（owner_player_id, registered_at DESC, room_id DESC）と同じ。続きは行の比べ方で取る（sqlboiler の条件の部品に行の比べ方が無いため、SQL を直接書く）
func (r *RoomReader) ListByPlayer(ctx context.Context, playerID uuid.UUID, condition query.ListRoomsCondition) ([]query.RoomListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	sql := "SELECT room_id, registered_at, metadata FROM rooms WHERE owner_player_id = $1 AND status <> $2"
	args := []any{playerID.String(), roomStatusClosed}
	if condition.After != nil {
		sql += " AND (registered_at, room_id) < ($3, $4)"
		args = append(args, condition.After.RegisteredAt, condition.After.RoomID)
	}
	args = append(args, condition.Limit)
	sql += fmt.Sprintf(" ORDER BY registered_at DESC, room_id DESC LIMIT $%d", len(args))

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type scanned struct {
		id           uuid.UUID
		registeredAt time.Time
		metadata     readmodel.RoomMetadata
	}
	var found []scanned
	for rows.Next() {
		var (
			item scanned
			raw  []byte
		)
		if err := rows.Scan(&item.id, &item.registeredAt, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.metadata); err != nil {
			return nil, fmt.Errorf("rooms.metadata of %s: %w", item.id, err)
		}
		found = append(found, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	metadatas := make([]readmodel.RoomMetadata, len(found))
	for i, item := range found {
		metadatas[i] = item.metadata
	}
	names, err := loadRoomNames(ctx, exec, metadatas)
	if err != nil {
		return nil, err
	}
	result := make([]query.RoomListRow, len(found))
	for i, item := range found {
		converted, err := toRoom(item.metadata, names)
		if err != nil {
			return nil, fmt.Errorf("rooms.metadata of %s: %w", item.id, err)
		}
		cursor := query.RoomListCursor{RegisteredAt: item.registeredAt, RoomID: item.id}
		result[i] = query.RoomListRow{Room: converted, Cursor: cursor}
	}
	return result, nil
}

// 持ち主で絞る。持ち主が違う行は無いのと同じ query.ErrRoomNotFound にする（ほかのプレイヤーの部屋の存在を明かさない）
func (r *RoomReader) Get(ctx context.Context, playerID uuid.UUID, roomID uuid.UUID) (query.Room, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.Room{}, err
	}
	row, err := models.Rooms(
		models.RoomWhere.RoomID.EQ(roomID.String()),
		models.RoomWhere.OwnerPlayerID.EQ(playerID.String()),
	).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.Room{}, query.ErrRoomNotFound
	}
	if err != nil {
		return query.Room{}, err
	}
	var metadata readmodel.RoomMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return query.Room{}, fmt.Errorf("rooms.metadata of %s: %w", row.RoomID, err)
	}
	names, err := loadRoomNames(ctx, exec, []readmodel.RoomMetadata{metadata})
	if err != nil {
		return query.Room{}, err
	}
	converted, err := toRoom(metadata, names)
	if err != nil {
		return query.Room{}, fmt.Errorf("rooms.metadata of %s: %w", row.RoomID, err)
	}
	return converted, nil
}

// 公開 ID から、見せる名前を引く表
type roomNames struct {
	avatars, decks, npcAvatars, npcDecks, npcMindsets, boards map[uuid.UUID]string
}

// どの表の metadata にも名前は name の項目にある
type namedMetadata struct {
	Name string `json:"name"`
}

func loadRoomNames(ctx context.Context, exec rdb.Executor, rooms []readmodel.RoomMetadata) (roomNames, error) {
	var avatarIDs, deckIDs, npcAvatarIDs, npcDeckIDs, npcMindsetIDs, boardIDs []uuid.UUID
	for _, metadata := range rooms {
		for _, seat := range metadata.Seats {
			if seat.Avatar == nil || seat.Deck == nil {
				continue
			}
			if seat.Kind == room.SeatKindNPC {
				npcAvatarIDs, npcDeckIDs = append(npcAvatarIDs, *seat.Avatar), append(npcDeckIDs, *seat.Deck)
				if seat.Mindset != nil {
					npcMindsetIDs = append(npcMindsetIDs, *seat.Mindset)
				}
			} else {
				avatarIDs, deckIDs = append(avatarIDs, *seat.Avatar), append(deckIDs, *seat.Deck)
			}
		}
		if metadata.MatchRules != nil {
			boardIDs = append(boardIDs, metadata.MatchRules.Board)
		}
	}
	var names roomNames
	var err error
	if names.avatars, err = namesOf(avatarIDs, func(args []any) ([]rowName, error) {
		rows, err := models.Avatars(qm.WhereIn(models.AvatarColumns.AvatarID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.Avatar) rowName { return rowName{row.AvatarID, row.Metadata} })
	}); err != nil {
		return roomNames{}, err
	}
	if names.decks, err = namesOf(deckIDs, func(args []any) ([]rowName, error) {
		rows, err := models.Decks(qm.WhereIn(models.DeckColumns.DeckID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.Deck) rowName { return rowName{row.DeckID, row.Metadata} })
	}); err != nil {
		return roomNames{}, err
	}
	if names.npcAvatars, err = namesOf(npcAvatarIDs, func(args []any) ([]rowName, error) {
		rows, err := models.NPCAvatars(qm.WhereIn(models.NPCAvatarColumns.NPCAvatarID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.NPCAvatar) rowName { return rowName{row.NPCAvatarID, row.Metadata} })
	}); err != nil {
		return roomNames{}, err
	}
	if names.npcDecks, err = namesOf(npcDeckIDs, func(args []any) ([]rowName, error) {
		rows, err := models.NPCDecks(qm.WhereIn(models.NPCDeckColumns.NPCDeckID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.NPCDeck) rowName { return rowName{row.NPCDeckID, row.Metadata} })
	}); err != nil {
		return roomNames{}, err
	}
	if names.npcMindsets, err = namesOf(npcMindsetIDs, func(args []any) ([]rowName, error) {
		rows, err := models.NPCMindsets(qm.WhereIn(models.NPCMindsetColumns.NPCMindsetID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.NPCMindset) rowName { return rowName{row.NPCMindsetID, row.Metadata} })
	}); err != nil {
		return roomNames{}, err
	}
	if names.boards, err = namesOf(boardIDs, func(args []any) ([]rowName, error) {
		rows, err := models.Boards(qm.WhereIn(models.BoardColumns.BoardID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.Board) rowName { return rowName{row.BoardID, row.Metadata} })
	}); err != nil {
		return roomNames{}, err
	}
	return names, nil
}

type rowName struct {
	id       string
	metadata []byte
}

func mapRows[R any](rows []R, err error, toRow func(R) rowName) ([]rowName, error) {
	if err != nil {
		return nil, err
	}
	result := make([]rowName, len(rows))
	for i, row := range rows {
		result[i] = toRow(row)
	}
	return result, nil
}

// 渡した公開 ID の名前を、表から 1 回で読む。無い行は結果に含めない（部屋に出すときに誤りにする）
func namesOf(ids []uuid.UUID, load func(args []any) ([]rowName, error)) (map[uuid.UUID]string, error) {
	names := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return names, nil
	}
	rows, err := load(uuidArgs(ids))
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		id, err := uuid.Parse(row.id)
		if err != nil {
			return nil, fmt.Errorf("room name source id %q: %w", row.id, err)
		}
		var metadata namedMetadata
		if err := json.Unmarshal(row.metadata, &metadata); err != nil {
			return nil, fmt.Errorf("metadata of %s: %w", id, err)
		}
		names[id] = metadata.Name
	}
	return names, nil
}

func toRoom(metadata readmodel.RoomMetadata, names roomNames) (query.Room, error) {
	status, err := toRoomStatus(metadata.Status)
	if err != nil {
		return query.Room{}, err
	}
	result := query.Room{ID: metadata.ID, Name: metadata.Name, Revision: metadata.Revision, Status: status, Match: metadata.Match, Matches: metadata.Matches}
	if result.Matches == nil {
		result.Matches = []uuid.UUID{}
	}
	result.Participants = []query.RoomParticipant{{PlayerID: metadata.Owner, Owner: true, Ready: slices.Contains(metadata.ReadyPlayers, metadata.Owner), Continue: continueOf(metadata, metadata.Owner)}}
	for _, seat := range metadata.Seats {
		kind, err := toRoomSeatKind(seat.Kind)
		if err != nil {
			return query.Room{}, err
		}
		converted := query.RoomSeat{Kind: kind, Player: seat.Player}
		if seat.Player != nil && !containsParticipant(result.Participants, *seat.Player) {
			result.Participants = append(result.Participants, query.RoomParticipant{PlayerID: *seat.Player, Ready: slices.Contains(metadata.ReadyPlayers, *seat.Player), Continue: continueOf(metadata, *seat.Player)})
		}
		if seat.Avatar != nil && seat.Deck != nil {
			avatarNames, deckNames := names.avatars, names.decks
			if kind == query.RoomSeatKindNPC {
				avatarNames, deckNames = names.npcAvatars, names.npcDecks
			}
			avatarName, ok := avatarNames[*seat.Avatar]
			if !ok {
				return query.Room{}, fmt.Errorf("avatar %s of a seat is not found", *seat.Avatar)
			}
			deckName, ok := deckNames[*seat.Deck]
			if !ok {
				return query.Room{}, fmt.Errorf("deck %s of a seat is not found", *seat.Deck)
			}
			converted.Avatar = &query.RoomNamed{ID: *seat.Avatar, Name: avatarName}
			converted.Deck = &query.RoomNamed{ID: *seat.Deck, Name: deckName}
			if seat.Mindset != nil {
				mindsetName, ok := names.npcMindsets[*seat.Mindset]
				if !ok {
					return query.Room{}, fmt.Errorf("mindset %s of a seat is not found", *seat.Mindset)
				}
				converted.Mindset = &query.RoomNamed{ID: *seat.Mindset, Name: mindsetName}
			}
		}
		result.Seats = append(result.Seats, converted)
	}
	if result.Seats == nil {
		result.Seats = []query.RoomSeat{}
	}
	if rules := metadata.MatchRules; rules != nil {
		boardName, ok := names.boards[rules.Board]
		if !ok {
			return query.Room{}, fmt.Errorf("board %s of match rules is not found", rules.Board)
		}
		boardNamed := query.RoomNamed{ID: rules.Board, Name: boardName}
		result.MatchRules = &query.RoomMatchRules{
			Board:       boardNamed,
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
			RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax, BaseLandValue: rules.BaseLandValue, LapBonusPerTerritory: rules.LapBonusPerTerritory,
		}
	}
	return result, nil
}

func continueOf(metadata readmodel.RoomMetadata, playerID uuid.UUID) *bool {
	for _, declaration := range metadata.NextDeclarations {
		if declaration.Player == playerID {
			value := declaration.Continue
			return &value
		}
	}
	return nil
}

// 対戦の公開 ID から、その対戦を始めた部屋と何戦目かを引く。索引（idx_rooms_matches）に合わせ、metadata -> 'matches' の包含で探す
// （部屋に列を足さないのは、1 つの部屋が対戦を何度も持つため。部屋の持ち主で絞らない:
// 対戦の参加者かは対戦の側が確かめ、ここは部屋の ID と順を返すだけ）
func (r *RoomReader) FindByMatch(ctx context.Context, matchID uuid.UUID) (uuid.UUID, int, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return uuid.Nil, 0, false, err
	}
	contained, err := json.Marshal([]uuid.UUID{matchID})
	if err != nil {
		return uuid.Nil, 0, false, err
	}
	var (
		roomID uuid.UUID
		raw    []byte
	)
	err = exec.QueryRowContext(ctx, "SELECT room_id, metadata -> 'matches' FROM rooms WHERE metadata -> 'matches' @> $1::jsonb", string(contained)).Scan(&roomID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, 0, false, nil
	}
	if err != nil {
		return uuid.Nil, 0, false, err
	}
	var matches []uuid.UUID
	if err := json.Unmarshal(raw, &matches); err != nil {
		return uuid.Nil, 0, false, fmt.Errorf("rooms.metadata matches of %s: %w", roomID, err)
	}
	index := slices.Index(matches, matchID)
	if index < 0 {
		return uuid.Nil, 0, false, fmt.Errorf("rooms.metadata matches of %s does not contain %s", roomID, matchID)
	}
	return roomID, index + 1, true, nil
}

// その人が作った部屋で始めた対戦か（部屋を作った人は、参加していない全員 NPC の対戦も見られるため。2026-11-17 オーナー。core#179）。
// 索引（idx_rooms_matches）に合わせ、FindByMatch と同じ包含で探す
func (r *RoomReader) OwnsMatch(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return false, err
	}
	contained, err := json.Marshal([]uuid.UUID{matchID})
	if err != nil {
		return false, err
	}
	var owns bool
	err = exec.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM rooms WHERE owner_player_id = $1 AND metadata -> 'matches' @> $2::jsonb)", playerID.String(), string(contained)).Scan(&owns)
	if err != nil {
		return false, err
	}
	return owns, nil
}

func toRoomStatus(status readmodel.RoomStatus) (query.RoomStatus, error) {
	switch status {
	case readmodel.RoomStatusDraft:
		return query.RoomStatusDraft, nil
	case readmodel.RoomStatusPreparing:
		return query.RoomStatusPreparing, nil
	case readmodel.RoomStatusPlaying:
		return query.RoomStatusPlaying, nil
	case readmodel.RoomStatusDecidingNext:
		return query.RoomStatusDecidingNext, nil
	case readmodel.RoomStatusPaused:
		return query.RoomStatusPaused, nil
	case readmodel.RoomStatusClosed:
		return query.RoomStatusClosed, nil
	default:
		return "", fmt.Errorf("unknown room status: %q", status)
	}
}

func toRoomSeatKind(kind room.SeatKind) (query.RoomSeatKind, error) {
	switch kind {
	case room.SeatKindPlayer:
		return query.RoomSeatKindPlayer, nil
	case room.SeatKindNPC:
		return query.RoomSeatKindNPC, nil
	default:
		return "", fmt.Errorf("unknown room seat kind: %q", kind)
	}
}

func containsParticipant(participants []query.RoomParticipant, id uuid.UUID) bool {
	for _, participant := range participants {
		if participant.PlayerID == id {
			return true
		}
	}
	return false
}

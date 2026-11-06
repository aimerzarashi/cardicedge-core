package graphql

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 部屋の操作に共通の、application 層のエラーから GraphQL のエラー型への写し。操作ごとに返してよい型は union が決めるので、
// 呼ぶ側が自分の結果の型へ型アサーションし、union に入っていない型は（application 層が返したのに写し忘れた）、黙って別の型にせず内部のエラーのまま返す。
// 写せなければ nil
func roomError(err error) any {
	switch {
	case errors.Is(err, command.ErrRoomNotFound):
		return &model.RoomNotFoundError{Message: "room not found"}
	case errors.Is(err, command.ErrRevisionConflict):
		return &model.RevisionConflictError{Message: "revision conflict"}
	case errors.Is(err, command.ErrRoomStatusNotAllowed):
		return &model.RoomStatusNotAllowedError{Message: "room operation not allowed in this status"}
	case errors.Is(err, command.ErrRoomSeatsNotDecided):
		return &model.RoomSeatsNotDecidedError{Message: "room seats not decided"}
	case errors.Is(err, command.ErrRoomConditionLocked):
		return &model.RoomConditionLockedError{Message: "room condition locked"}
	case errors.Is(err, command.ErrRoomNotResponsible):
		return &model.RoomNotResponsibleError{Message: "room operation is not yours"}
	case errors.Is(err, command.ErrRoomAlreadyReady):
		return &model.RoomAlreadyReadyError{Message: "room already ready"}
	case errors.Is(err, command.ErrRoomAlreadyDeclaredNext):
		return &model.RoomAlreadyDeclaredNextError{Message: "room already declared next"}
	case errors.Is(err, command.ErrRoomConditionsNotDecided):
		return &model.RoomConditionsNotDecidedError{Message: "room conditions not decided"}
	}
	return nil
}

func toRoomSeatKinds(kinds []model.RoomSeatKind) ([]command.RoomSeatKind, error) {
	seats := make([]command.RoomSeatKind, len(kinds))
	for i, kind := range kinds {
		switch kind {
		case model.RoomSeatKindPlayer:
			seats[i] = command.RoomSeatKindPlayer
		case model.RoomSeatKindNpc:
			seats[i] = command.RoomSeatKindNPC
		default:
			return nil, fmt.Errorf("graphql: unknown room seat kind: %q", kind)
		}
	}
	return seats, nil
}

func fromQueryRoom(room query.Room) (*model.Room, error) {
	status, err := fromRoomStatus(room.Status)
	if err != nil {
		return nil, err
	}
	participants := make([]*model.RoomParticipant, len(room.Participants))
	for i, participant := range room.Participants {
		participants[i] = &model.RoomParticipant{PlayerID: participant.PlayerID, Owner: participant.Owner, Ready: participant.Ready, Continues: participant.Continue}
	}
	seats := make([]*model.RoomSeat, len(room.Seats))
	for i, seat := range room.Seats {
		kind, err := fromRoomSeatKind(seat.Kind)
		if err != nil {
			return nil, err
		}
		seats[i] = &model.RoomSeat{Kind: kind, PlayerID: seat.Player, Avatar: fromRoomNamed(seat.Avatar), Deck: fromRoomNamed(seat.Deck)}
		// 投影が遅れて席の数がずれたときは、押せない（false）にする。操作は revision で弾かれ、読み直せば直る
		if capabilities := room.Capabilities; capabilities != nil {
			changeable := i < len(capabilities.ChangeSeatChoice) && capabilities.ChangeSeatChoice[i]
			seats[i].ChoiceChangeable = &changeable
		}
	}
	matches := make([]*model.RoomMatch, len(room.Matches))
	for i, id := range room.Matches {
		matches[i] = &model.RoomMatch{ID: id, Number: i + 1}
	}
	converted := &model.Room{ID: room.ID, Name: room.Name, Revision: int(room.Revision), Status: status, Participants: participants, Seats: seats, Matches: matches}
	if capabilities := room.Capabilities; capabilities != nil {
		// 対戦は詳細だけが返す（一覧の部屋ごとに対戦を読み直さないため。できる操作と同じ）
		converted.MatchID = room.Match
		converted.Capabilities = &model.RoomCapabilities{
			ChangeSeats: capabilities.ChangeSeats, ChangeMatchRules: capabilities.ChangeMatchRules, Publish: capabilities.Publish,
			DeclareReady: capabilities.DeclareReady, DeclareNext: capabilities.DeclareNext, Resume: capabilities.Resume, Close: capabilities.Close, Rename: capabilities.Rename,
		}
	}
	if rules := room.MatchRules; rules != nil {
		converted.MatchRules = &model.RoomMatchRules{
			Board:       fromRoomNamed(&rules.Board),
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
			RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax,
		}
	}
	return converted, nil
}

func fromRoomNamed(named *query.RoomNamed) *model.RoomNamed {
	if named == nil {
		return nil
	}
	return &model.RoomNamed{ID: named.ID, Name: named.Name}
}

// 種類の対応を明示する（read model の値が増えて写し忘れたら、黙って落とさずエラーにする）
func fromRoomStatus(status query.RoomStatus) (model.RoomStatus, error) {
	switch status {
	case query.RoomStatusDraft:
		return model.RoomStatusDraft, nil
	case query.RoomStatusPreparing:
		return model.RoomStatusPreparing, nil
	case query.RoomStatusPlaying:
		return model.RoomStatusPlaying, nil
	case query.RoomStatusDecidingNext:
		return model.RoomStatusDecidingNext, nil
	case query.RoomStatusPaused:
		return model.RoomStatusPaused, nil
	case query.RoomStatusClosed:
		return model.RoomStatusClosed, nil
	default:
		return "", fmt.Errorf("graphql: unknown room status: %q", status)
	}
}

func fromRoomSeatKind(kind query.RoomSeatKind) (model.RoomSeatKind, error) {
	switch kind {
	case query.RoomSeatKindPlayer:
		return model.RoomSeatKindPlayer, nil
	case query.RoomSeatKindNPC:
		return model.RoomSeatKindNpc, nil
	default:
		return "", fmt.Errorf("graphql: unknown room seat kind: %q", kind)
	}
}

func toRoomsConnection(connection pagination.Connection[query.Room]) (*model.RoomsConnection, error) {
	edges := make([]*model.RoomsEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryRoom(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.RoomsEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.RoomsConnection{Edges: edges, PageInfo: pageInfo}, nil
}

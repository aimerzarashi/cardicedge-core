package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// 書くときに、イベントが内部 ID で持つ部屋・人・選んだもの・ボードを公開用の ID に引き直すため、引き方を持つ（UnlockWriter と同じ）
type RoomWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewRoomWriter(entityRefs domain.EntityRefResolver) *RoomWriter {
	return &RoomWriter{entityRefs: entityRefs}
}

func (w *RoomWriter) Upsert(ctx context.Context, state readmodel.Room) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	refs := roomRefs(state)
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	lookup := func(ref domain.EntityRef) (uuid.UUID, error) {
		publicID, ok := publicIDs[ref]
		if !ok {
			return uuid.Nil, fmt.Errorf("repository: %s %d of room has no public id", ref.EntityType(), ref.ID())
		}
		return publicID, nil
	}
	metadata, err := roomMetadata(state, lookup)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.Room{
		RoomID:        metadata.ID.String(),
		OwnerPlayerID: metadata.Owner.String(),
		Status:        string(state.Status),
		RegisteredAt:  state.RegisteredAt,
		Metadata:      raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.RoomColumns.RoomID},
		boil.Whitelist(models.RoomColumns.OwnerPlayerID, models.RoomColumns.Status, models.RoomColumns.RegisteredAt, models.RoomColumns.Metadata),
		boil.Infer(),
	)
}

// 部屋を消すイベントは無いため呼ばれない（投影の宣言に要るので置く）。行が無くてもエラーにしない
func (w *RoomWriter) Delete(ctx context.Context, state readmodel.Room) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	ref := domain.NewEntityRef(room.Types().EntityType, state.Room.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return nil
	}
	_, err = models.Rooms(models.RoomWhere.RoomID.EQ(publicID.String())).DeleteAll(ctx, exec)
	return err
}

// 公開 ID に引き直す内部 ID
func roomRefs(state readmodel.Room) []domain.EntityRef {
	refs := []domain.EntityRef{
		domain.NewEntityRef(room.Types().EntityType, state.Room.Int64()),
		domain.NewEntityRef(player.Types().EntityType, state.Owner.Int64()),
		domain.NewEntityRef(player.Types().EntityType, state.UpdatedBy.Int64()),
	}
	for _, seat := range state.Seats {
		if seat.Kind == room.SeatKindPlayer {
			refs = append(refs, domain.NewEntityRef(player.Types().EntityType, seat.Player.Int64()))
		}
		if seat.Choice == nil {
			continue
		}
		if seat.Choice.Player != nil {
			refs = append(refs,
				domain.NewEntityRef(avatar.Types().EntityType, seat.Choice.Player.Avatar.Int64()),
				domain.NewEntityRef(deck.Types().EntityType, seat.Choice.Player.Deck.Int64()))
		}
		if seat.Choice.NPC != nil {
			refs = append(refs,
				domain.NewEntityRef(npcavatar.Types().EntityType, seat.Choice.NPC.Avatar.Int64()),
				domain.NewEntityRef(npcdeck.Types().EntityType, seat.Choice.NPC.Deck.Int64()),
				domain.NewEntityRef(npcmindset.Types().EntityType, seat.Choice.NPC.Mindset.Int64()))
		}
	}
	if state.MatchRules != nil {
		refs = append(refs, domain.NewEntityRef(board.Types().EntityType, state.MatchRules.Board.Int64()))
	}
	for _, ready := range state.Ready {
		refs = append(refs, domain.NewEntityRef(player.Types().EntityType, ready.Int64()))
	}
	if state.Match != 0 {
		refs = append(refs, domain.NewEntityRef(match.Types().EntityType, state.Match.Int64()))
	}
	for _, started := range state.Matches {
		refs = append(refs, domain.NewEntityRef(match.Types().EntityType, started.Int64()))
	}
	for _, declaration := range state.NextDeclarations {
		refs = append(refs, domain.NewEntityRef(player.Types().EntityType, declaration.Player.Int64()))
	}
	return refs
}

func roomMetadata(state readmodel.Room, lookup func(domain.EntityRef) (uuid.UUID, error)) (readmodel.RoomMetadata, error) {
	var errs []error
	publicID := func(entityType domain.EntityType, id int64) uuid.UUID {
		value, err := lookup(domain.NewEntityRef(entityType, id))
		errs = append(errs, err)
		return value
	}
	metadata := state.Metadata()
	metadata.ID = publicID(room.Types().EntityType, state.Room.Int64())
	metadata.Owner = publicID(player.Types().EntityType, state.Owner.Int64())
	// 部屋を作るのは持ち主（デッキ・アバターと同じく、登録の操作をした人）
	metadata.RegisteredBy = metadata.Owner
	metadata.UpdatedBy = publicID(player.Types().EntityType, state.UpdatedBy.Int64())
	metadata.Seats = make([]readmodel.RoomSeatMetadata, len(state.Seats))
	for i, seat := range state.Seats {
		seatMetadata := readmodel.RoomSeatMetadata{Kind: seat.Kind}
		if seat.Kind == room.SeatKindPlayer {
			seatPlayer := publicID(player.Types().EntityType, seat.Player.Int64())
			seatMetadata.Player = &seatPlayer
		}
		if choice := seat.Choice; choice != nil {
			var avatarID, deckID uuid.UUID
			if choice.Player != nil {
				avatarID = publicID(avatar.Types().EntityType, choice.Player.Avatar.Int64())
				deckID = publicID(deck.Types().EntityType, choice.Player.Deck.Int64())
			}
			if choice.NPC != nil {
				avatarID = publicID(npcavatar.Types().EntityType, choice.NPC.Avatar.Int64())
				deckID = publicID(npcdeck.Types().EntityType, choice.NPC.Deck.Int64())
			}
			seatMetadata.Avatar, seatMetadata.Deck = &avatarID, &deckID
			if choice.NPC != nil {
				mindsetID := publicID(npcmindset.Types().EntityType, choice.NPC.Mindset.Int64())
				seatMetadata.Mindset = &mindsetID
			}
		}
		metadata.Seats[i] = seatMetadata
	}
	if metadata.MatchRules != nil {
		metadata.MatchRules.Board = publicID(board.Types().EntityType, state.MatchRules.Board.Int64())
	}
	metadata.ReadyPlayers = make([]uuid.UUID, len(state.Ready))
	for i, ready := range state.Ready {
		metadata.ReadyPlayers[i] = publicID(player.Types().EntityType, ready.Int64())
	}
	if state.Match != 0 {
		matchID := publicID(match.Types().EntityType, state.Match.Int64())
		metadata.Match = &matchID
	}
	metadata.Matches = make([]uuid.UUID, len(state.Matches))
	for i, started := range state.Matches {
		metadata.Matches[i] = publicID(match.Types().EntityType, started.Int64())
	}
	metadata.NextDeclarations = make([]readmodel.RoomNextDeclarationMetadata, len(state.NextDeclarations))
	for i, declaration := range state.NextDeclarations {
		metadata.NextDeclarations[i] = readmodel.RoomNextDeclarationMetadata{Player: publicID(player.Types().EntityType, declaration.Player.Int64()), Continue: declaration.Continue}
	}
	if err := errors.Join(errs...); err != nil {
		return readmodel.RoomMetadata{}, err
	}
	return metadata, nil
}

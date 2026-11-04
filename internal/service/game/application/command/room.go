package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

// 公開 ID を game の内部 ID に引き直す（席の選択・対戦ルールのボードは、同じ境界の内部 ID で持つ）
type RoomReferences struct {
	Avatars    domain.Identifier[avatar.ID]
	Decks      domain.Identifier[deck.ID]
	NPCAvatars domain.Identifier[npcavatar.ID]
	NPCDecks   domain.Identifier[npcdeck.ID]
	Boards     domain.Identifier[board.ID]
}

// 席・対戦ルールに選べるもの。画面の選択肢を出す query（Player.avatars・decks・npcAvatars・boards）と同じ条件を、同じハンドラーを通して
// 調べる実装（query.RoomSelectable）を internal/service/game が渡す。選んだ時（changeRoomSeatChoice・changeRoomMatchRules）に 1 回だけ確かめ、
// 準備完了の時には確かめ直さない（選んだ後に使用不可になっても、対戦は始めた時点の写しで進むため）
type RoomSelectable interface {
	HasAvatar(ctx context.Context, playerID uuid.UUID, avatarID uuid.UUID) (bool, error)
	HasDeck(ctx context.Context, playerID uuid.UUID, deckID uuid.UUID) (bool, error)
	HasNPCAvatar(ctx context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID) (bool, error)
	HasNPCDeck(ctx context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID, npcDeckID uuid.UUID) (bool, error)
	HasBoard(ctx context.Context, playerID uuid.UUID, boardID uuid.UUID) (bool, error)
}

type RoomHandler struct {
	eventStore domain.EventStore[room.ID, room.Event, room.State]
	identifier domain.Identifier[room.ID]
	matches    domain.Identifier[match.ID]
	references RoomReferences
	entityRefs domain.EntityRefResolver
	selectable RoomSelectable
	getRules   rules.GetRulesFunc
	now        func() time.Time
}

func NewRoomHandler(
	eventStore domain.EventStore[room.ID, room.Event, room.State],
	identifier domain.Identifier[room.ID],
	matches domain.Identifier[match.ID],
	references RoomReferences,
	entityRefs domain.EntityRefResolver,
	selectable RoomSelectable,
	getRules rules.GetRulesFunc,
	now func() time.Time,
) *RoomHandler {
	return &RoomHandler{
		eventStore: eventStore, identifier: identifier, matches: matches, references: references, entityRefs: entityRefs,
		selectable: selectable, getRules: getRules, now: now,
	}
}

// 席の種類。presentation 層にドメインの型を見せないため、application 層の型で受け取る
type RoomSeatKind string

const (
	RoomSeatKindPlayer RoomSeatKind = "player"
	RoomSeatKindNPC    RoomSeatKind = "npc"
)

// 対戦ルールの入力。ボードの公開 ID と、ボードの既定のルールと同じ名前の 7 つの値
type RoomMatchRulesInput struct {
	BoardID         uuid.UUID
	TargetMagic     int
	StartingMagic   int
	LapBonus        int
	CheckpointBonus int
	RoundLimit      int
	DiceMin         int
	DiceMax         int
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（内容は返さない。画面は詳細を読み直し、詳細の revision を持つ）
type ChangedRoom struct {
	ID       uuid.UUID
	Revision int64
}

// 入力の名前を部屋の名前にする。誤りは input.Errors（空は Required、長すぎは TooLong。項目名 name）
func newRoomName(value string) (room.Name, error) {
	name, err := room.NewName(value)
	switch {
	case errors.Is(err, room.ErrNameEmpty):
		return "", input.Errors{&input.Required{Field: "name"}}
	case errors.Is(err, room.ErrNameTooLong):
		return "", input.Errors{&input.TooLong{Field: "name", Max: room.MaxNameLength}}
	case err != nil:
		return "", err
	}
	return name, nil
}

// context のプレイヤー（認証ミドルウェアが載せたもの）を持ち主とする部屋を作る。席は 0 で、作った人はプレイヤーの席に座る。
// 名前はほかの部屋と同じでもよい（一意にしない）。名前の誤りは input.Errors。
// 登録済みのプレイヤーが載っていなければ ErrPlayerNotRegistered
func (c *RoomHandler) Create(ctx context.Context, name string) (ChangedRoom, error) {
	owner, ok := authentication.Player(ctx)
	if !ok {
		return ChangedRoom{}, ErrPlayerNotRegistered
	}
	roomName, err := newRoomName(name)
	if err != nil {
		return ChangedRoom{}, err
	}
	return rdb.Transact(ctx, func(ctx context.Context) (ChangedRoom, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedRoom{}, err
		}
		events, err := room.Decide(room.InitialState(), &room.CreateCommand{ID: id, Owner: owner, Name: roomName}, c.now())
		if err != nil {
			return ChangedRoom{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedRoom{}, err
		}
		return ChangedRoom{ID: publicID, Revision: events[len(events)-1].GetMeta().AggregateVersion}, nil
	})
}

// 席の並びを決める（公開前だけ）。席の並びを変えたら、席ごとの選択はすべて外れる。
// 入力の誤りは input.Errors（段階ごと。席の数が全体のルールの範囲の外なら OutOfRange、範囲の中でプレイヤーの席の数が決まりと違えば
// CountMismatch。項目名は seats）。ほかのエラーは ErrRoomNotFound（無い・入っていない部屋）、ErrRevisionConflict、
// ErrRoomStatusNotAllowed、ErrRoomNotResponsible
func (c *RoomHandler) ChangeSeats(ctx context.Context, id uuid.UUID, revision int64, seats []RoomSeatKind) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		gameRules, found, err := c.getRules(ctx)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, rules.ErrNotPublished
		}
		kinds := make([]room.SeatKind, len(seats))
		for i, seat := range seats {
			if kinds[i], err = room.NewSeatKind(string(seat)); err != nil {
				return nil, input.Errors{&input.InvalidFormat{Field: fmt.Sprintf("seats.%d", i)}}
			}
		}
		minPlayers, maxPlayers := gameRules.Match.MinPlayers, gameRules.Match.MaxPlayers
		seatKinds, err := room.NewSeatKinds(kinds, minPlayers, maxPlayers)
		switch {
		case errors.Is(err, room.ErrTooFewSeats), errors.Is(err, room.ErrTooManySeats):
			return nil, input.Errors{&input.OutOfRange{Field: "seats", Min: minPlayers.Int(), Max: maxPlayers.Int()}}
		case errors.Is(err, room.ErrPlayerSeatCount):
			return nil, input.Errors{&input.CountMismatch{Field: "seats", Expected: room.PlayerSeatCount}}
		case err != nil:
			return nil, err
		}
		return &room.ChangeSeatsCommand{ChangedBy: actor, Seats: seatKinds}, nil
	})
}

// 対戦ルールを決める（公開前、または公開前に決めていなければ準備中）。決めるのは作った人。
// 入力の誤りは input.Errors（まとめて返す）。7 つの値の誤りは board.NewDefaultRules の決まりで、項目名は GraphQL の入力の項目名
// （targetMagic 等。TooSmall・NotGreaterThan）。ボードが解放した公開中のものでなければ NotFound（項目名 boardId）。
// ほかのエラーは ChangeSeats と同じ。公開前に決めたものは ErrRoomConditionLocked、準備完了の申告の後は ErrRoomAlreadyReady
func (c *RoomHandler) ChangeMatchRules(ctx context.Context, id uuid.UUID, revision int64, rulesInput RoomMatchRulesInput) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		invalid := valuesToInput(rulesInput)
		boardID, boardInput, err := c.selectBoard(ctx, current, rulesInput.BoardID)
		if err != nil {
			return nil, err
		}
		invalid = append(invalid, boardInput...)
		if len(invalid) > 0 {
			return nil, invalid
		}
		matchRules, err := room.NewMatchRules(boardID, rulesInput.TargetMagic, rulesInput.StartingMagic, rulesInput.LapBonus,
			rulesInput.CheckpointBonus, rulesInput.RoundLimit, rulesInput.DiceMin, rulesInput.DiceMax)
		if err != nil {
			return nil, err
		}
		return &room.ChangeMatchRulesCommand{ChangedBy: actor, MatchRules: matchRules}, nil
	})
}

// 7 つの値の決まりは board.NewDefaultRules にあり、ここは違反を入力の項目に写すだけ（違反が無ければ空）
func valuesToInput(rulesInput RoomMatchRulesInput) input.Errors {
	_, err := board.NewDefaultRules(rulesInput.TargetMagic, rulesInput.StartingMagic, rulesInput.LapBonus, rulesInput.CheckpointBonus,
		rulesInput.RoundLimit, rulesInput.DiceMin, rulesInput.DiceMax)
	if err == nil {
		return nil
	}
	invalid := input.Errors{}
	if errors.Is(err, board.ErrTargetMagicTooSmall) {
		invalid = append(invalid, &input.TooSmall{Field: "targetMagic", Min: 1})
	}
	if errors.Is(err, board.ErrStartingMagicTooSmall) {
		invalid = append(invalid, &input.TooSmall{Field: "startingMagic", Min: 0})
	}
	if errors.Is(err, board.ErrLapBonusTooSmall) {
		invalid = append(invalid, &input.TooSmall{Field: "lapBonus", Min: 0})
	}
	if errors.Is(err, board.ErrCheckpointBonusTooSmall) {
		invalid = append(invalid, &input.TooSmall{Field: "checkpointBonus", Min: 0})
	}
	if errors.Is(err, board.ErrTargetMagicNotGreaterThanStartingMagic) {
		invalid = append(invalid, &input.NotGreaterThan{Field: "targetMagic", Than: "startingMagic"})
	}
	if errors.Is(err, board.ErrRoundLimitTooSmall) {
		invalid = append(invalid, &input.TooSmall{Field: "roundLimit", Min: 1})
	}
	if errors.Is(err, board.ErrDiceMinTooSmall) {
		invalid = append(invalid, &input.TooSmall{Field: "diceMin", Min: 1})
	}
	if errors.Is(err, board.ErrDiceMaxNotGreaterThanDiceMin) {
		invalid = append(invalid, &input.NotGreaterThan{Field: "diceMax", Than: "diceMin"})
	}
	if len(invalid) == 0 {
		// 理由を domain に足したのに、ここに写し方を足し忘れた（黙って通さない）
		return input.Errors{&input.InvalidFormat{Field: "targetMagic"}}
	}
	return invalid
}

// ボードが、作った人が解放した公開中のものか（Player.boards と同じ条件）を確かめ、内部 ID に引き直す
func (c *RoomHandler) selectBoard(ctx context.Context, current room.Room, boardPublicID uuid.UUID) (board.ID, input.Errors, error) {
	ownerPublicID, err := c.playerPublicID(ctx, current.Owner)
	if err != nil {
		return 0, nil, err
	}
	selectable, err := c.selectable.HasBoard(ctx, ownerPublicID, boardPublicID)
	if err != nil {
		return 0, nil, err
	}
	if !selectable {
		return 0, input.Errors{&input.NotFound{Field: "boardId"}}, nil
	}
	boardID, err := c.references.Boards.Resolve(ctx, boardPublicID)
	return boardID, nil, err
}

// 席のアバターとデッキを選ぶ（公開前、または公開前に決めていなければ準備中）。席の種類で、選ぶものが変わる:
// プレイヤーの席は座っている人のアバターとデッキ（Player.avatars・decks に出るもの）、NPC の席は作った人が解放した公開中の NPC のアバターと、
// そのアバターに割り当てた公開中のデッキ（Player.npcAvatars に出るもの）。選んだ時の 1 回だけ確かめる。
// 席が決まっていなければ ErrRoomSeatsNotDecided。入力の誤りは input.Errors（段階ごと。席が範囲の外なら OutOfRange・項目名 seat。
// 次に、アバターとデッキが選べるものに無ければ NotFound・項目名 avatarId・deckId。NPC の席は、アバターが選べるときだけデッキを確かめる）。
// ほかのエラーは ChangeMatchRules と同じ
func (c *RoomHandler) ChangeSeatChoice(ctx context.Context, id uuid.UUID, revision int64, seat int, avatarID uuid.UUID, deckID uuid.UUID) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		if len(current.Seats) == 0 {
			return nil, room.ErrSeatsNotDecided
		}
		seatIndex, err := room.NewSeatIndex(seat)
		if err != nil || seatIndex.Int() >= len(current.Seats) {
			return nil, input.Errors{&input.OutOfRange{Field: "seat", Min: 0, Max: len(current.Seats) - 1}}
		}
		target := current.Seats[seatIndex].Seat
		var choice room.SeatChoice
		if target.Kind == room.SeatKindPlayer {
			choice, err = c.choosePlayerSeat(ctx, target.Player, avatarID, deckID)
		} else {
			choice, err = c.chooseNPCSeat(ctx, current.Owner, avatarID, deckID)
		}
		if err != nil {
			return nil, err
		}
		return &room.ChangeSeatChoiceCommand{ChangedBy: actor, Seat: seatIndex, Choice: choice}, nil
	})
}

func (c *RoomHandler) choosePlayerSeat(ctx context.Context, seatPlayer player.ID, avatarPublicID uuid.UUID, deckPublicID uuid.UUID) (room.SeatChoice, error) {
	playerPublicID, err := c.playerPublicID(ctx, seatPlayer)
	if err != nil {
		return room.SeatChoice{}, err
	}
	invalid := input.Errors{}
	hasAvatar, err := c.selectable.HasAvatar(ctx, playerPublicID, avatarPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	if !hasAvatar {
		invalid = append(invalid, &input.NotFound{Field: "avatarId"})
	}
	hasDeck, err := c.selectable.HasDeck(ctx, playerPublicID, deckPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	if !hasDeck {
		invalid = append(invalid, &input.NotFound{Field: "deckId"})
	}
	if len(invalid) > 0 {
		return room.SeatChoice{}, invalid
	}
	avatarID, err := c.references.Avatars.Resolve(ctx, avatarPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	deckID, err := c.references.Decks.Resolve(ctx, deckPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	return room.NewPlayerSeatChoice(avatarID, deckID), nil
}

func (c *RoomHandler) chooseNPCSeat(ctx context.Context, owner player.ID, avatarPublicID uuid.UUID, deckPublicID uuid.UUID) (room.SeatChoice, error) {
	ownerPublicID, err := c.playerPublicID(ctx, owner)
	if err != nil {
		return room.SeatChoice{}, err
	}
	hasAvatar, err := c.selectable.HasNPCAvatar(ctx, ownerPublicID, avatarPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	if !hasAvatar {
		return room.SeatChoice{}, input.Errors{&input.NotFound{Field: "avatarId"}}
	}
	hasDeck, err := c.selectable.HasNPCDeck(ctx, ownerPublicID, avatarPublicID, deckPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	if !hasDeck {
		return room.SeatChoice{}, input.Errors{&input.NotFound{Field: "deckId"}}
	}
	npcAvatarID, err := c.references.NPCAvatars.Resolve(ctx, avatarPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	npcDeckID, err := c.references.NPCDecks.Resolve(ctx, deckPublicID)
	if err != nil {
		return room.SeatChoice{}, err
	}
	return room.NewNPCSeatChoice(npcAvatarID, npcDeckID), nil
}

// 公開する（公開前だけ。作った人だけ）。席の並びが決まっていなければ ErrRoomSeatsNotDecided。対戦ルールと席の選択は、決まっていなくても公開できる
// （公開前に決めなかったものを準備中に決める）。プレイヤーの席が定員なら、そのまま準備中に進む。ほかのエラーは ChangeSeats と同じ
func (c *RoomHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		return &room.PublishCommand{PublishedBy: actor}, nil
	})
}

// context のプレイヤーが準備完了を申告する（準備中だけ。自分の担当の分がすべて決まっているとき）。
// 担当の分に決まっていないものがあれば ErrRoomConditionsNotDecided、すでに申告していれば ErrRoomAlreadyReady。
// 選んだ値が使えるかは、選んだ時に確かめているので、ここでは確かめ直さない。ほかのエラーは ChangeSeats と同じ。
// 最後の準備完了なら、同じトランザクションで対戦の ID を取って渡し、部屋が対戦中になる（対戦そのものはポリシーが作る）。
// ID を取るのは DB の仕事で Decide の中ではできず、取るかどうかはドメイン（StartsMatchOnReady）が決める。
// 最後でない準備完了では取らない（使わない ID で domain_entities の行を増やさないため）
func (c *RoomHandler) DeclareReady(ctx context.Context, id uuid.UUID, revision int64) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		cmd := &room.DeclareReadyCommand{Player: actor}
		if current.StartsMatchOnReady(actor) {
			matchID, _, err := c.matches.Issue(ctx)
			if err != nil {
				return nil, err
			}
			cmd.Match = matchID
		}
		return cmd, nil
	})
}

// 部屋の名前を変える（作った人だけ。閉じる前まで。公開の後にも変えられる）。同じ名前なら何も変えず成功する。
// 名前の誤りは input.Errors（Create と同じ）。閉じた部屋は ErrRoomStatusNotAllowed。ほかのエラーは ChangeSeats と同じ
func (c *RoomHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		roomName, err := newRoomName(name)
		if err != nil {
			return nil, err
		}
		return &room.RenameCommand{RenamedBy: actor, Name: roomName}, nil
	})
}

// 部屋を閉じる（作った人だけ。閉じた部屋を閉じ直しても成功する）。ほかのエラーは ChangeSeats と同じ
func (c *RoomHandler) Close(ctx context.Context, id uuid.UUID, revision int64) (ChangedRoom, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error) {
		return &room.CloseCommand{ClosedBy: actor}, nil
	})
}

// 部屋を読み、入っている人であることと revision を確かめてから、build が作ったコマンドを動かして保存する。
// 操作する人は context のプレイヤーで、build がコマンドに入れる。作った人だけの操作かどうか・担当の人かどうかは、ドメインが確かめる
func (c *RoomHandler) change(ctx context.Context, publicID uuid.UUID, revision int64, build func(ctx context.Context, current room.Room, actor player.ID) (room.Command, error)) (ChangedRoom, error) {
	actor, ok := authentication.Player(ctx)
	if !ok {
		return ChangedRoom{}, ErrPlayerNotRegistered
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedRoom, error) {
		id, err := c.identifier.Resolve(ctx, publicID)
		if errors.Is(err, domain.ErrEntityNotFound) {
			return ChangedRoom{}, ErrRoomNotFound
		}
		if err != nil {
			return ChangedRoom{}, err
		}
		state, err := c.eventStore.Load(ctx, id)
		if err != nil {
			return ChangedRoom{}, err
		}
		current, ok := state.(room.Room)
		if !ok || !current.HasParticipant(actor) {
			return ChangedRoom{}, ErrRoomNotFound
		}
		// revision を Load の直後に比べる理由は studio の PresetDeckHandler.load と同じ
		if current.Version != revision {
			return ChangedRoom{}, ErrRevisionConflict
		}
		cmd, err := build(ctx, current, actor)
		if err != nil {
			return ChangedRoom{}, err
		}
		events, err := room.Decide(current, cmd, c.now())
		if err != nil {
			return ChangedRoom{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedRoom{}, err
		}
		// 変わらない操作（同じ席の並び等）はイベントが無く、版が進まない
		version := current.Version
		if len(events) > 0 {
			version = events[len(events)-1].GetMeta().AggregateVersion
		}
		return ChangedRoom{ID: publicID, Revision: version}, nil
	})
	return changed, mapRoomError(err)
}

func (c *RoomHandler) playerPublicID(ctx context.Context, id player.ID) (uuid.UUID, error) {
	ref := domain.NewEntityRef(id.EntityType(), id.Int64())
	publicIDs, err := c.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return uuid.Nil, err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return uuid.Nil, errors.New("command: player has no public id")
	}
	return publicID, nil
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapRoomError(err error) error {
	switch {
	case errors.Is(err, room.ErrStatusNotAllowed):
		return ErrRoomStatusNotAllowed
	case errors.Is(err, room.ErrSeatsNotDecided):
		return ErrRoomSeatsNotDecided
	case errors.Is(err, room.ErrConditionLocked):
		return ErrRoomConditionLocked
	case errors.Is(err, room.ErrNotOwner), errors.Is(err, room.ErrNotResponsible), errors.Is(err, room.ErrNotParticipant):
		return ErrRoomNotResponsible
	case errors.Is(err, room.ErrAlreadyReady):
		return ErrRoomAlreadyReady
	case errors.Is(err, room.ErrConditionsNotDecided):
		return ErrRoomConditionsNotDecided
	}
	return revisionConflict(err)
}

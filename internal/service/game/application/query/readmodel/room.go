package readmodel

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
)

// rooms.status の値。domain の room.Status を使わず値を持つのは、read model の表の値は集約の型が変わっても変わらないようにするため
type RoomStatus string

const (
	RoomStatusDraft     RoomStatus = "draft"
	RoomStatusPreparing RoomStatus = "preparing"
	RoomStatusPlaying   RoomStatus = "playing"
	// 対戦が終わり、続けるか終えるかを意思表示する間
	RoomStatusDecidingNext RoomStatus = "deciding_next"
	// 対戦の後に誰かが終えた。一覧に残り、再開すると準備中に戻る
	RoomStatusPaused RoomStatus = "paused"
	RoomStatusClosed RoomStatus = "closed"
)

// DB の rooms.status など、外から来た値を状態にする
func NewRoomStatus(value string) (RoomStatus, error) {
	status := RoomStatus(value)
	switch status {
	case RoomStatusDraft, RoomStatusPreparing, RoomStatusPlaying, RoomStatusDecidingNext, RoomStatusPaused, RoomStatusClosed:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown room status: %q", value)
	}
}

// rooms の 1 行。部屋・持ち主・席に座る人・選んだもの・対戦ルールのボードは、イベントが内部 ID で持つ（イベントに公開 ID は無い）ため、
// 内部 ID で持ち、書くときに repository が公開 ID に引き直す
type Room struct {
	Room       room.ID
	Owner      player.ID
	Name       room.Name
	Status     RoomStatus
	Revision   int64
	Seats      []RoomSeat
	MatchRules *room.MatchRules
	// 準備完了を申告した人
	Ready []player.ID
	// 今の対戦。始まるまでは 0
	Match match.ID
	// この部屋で始めた対戦（始めた順）。部屋の MatchStartedEvent だけから作り、対戦のイベントは購読しない（結果は対戦が持つ）
	Matches []match.ID
	// 対戦の後の意思表示（次を決める間だけ。準備中に戻ったら空に戻す）
	NextDeclarations []RoomNextDeclaration
	RegisteredAt     time.Time
	UpdatedAt        time.Time
	// 最後に操作した人
	UpdatedBy player.ID
}

// 席 1 つ。選択は、まだ決まっていなければ nil
type RoomSeat struct {
	Kind   room.SeatKind
	Player player.ID
	Choice *room.SeatChoice
}

// 対戦の後の意思表示 1 つ。続けるなら Continue
type RoomNextDeclaration struct {
	Player   player.ID
	Continue bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。表示に使う ID は公開 ID で持つ
type RoomMetadata struct {
	// ID・Owner・Seats・MatchRules.Board・ReadyPlayers・Match・RegisteredBy・UpdatedBy は空のまま。書くときに repository が公開 ID を入れる
	ID           uuid.UUID               `json:"id"`
	Name         string                  `json:"name"`
	Status       RoomStatus              `json:"status"`
	Revision     int64                   `json:"revision"`
	Owner        uuid.UUID               `json:"owner"`
	Seats        []RoomSeatMetadata      `json:"seats"`
	MatchRules   *RoomMatchRulesMetadata `json:"match_rules"`
	ReadyPlayers []uuid.UUID             `json:"ready_players"`
	// 始まった対戦の公開 ID。始まるまでは null。空のまま、書くときに repository が公開 ID を入れる
	Match *uuid.UUID `json:"match"`
	// この部屋で始めた対戦の公開 ID（始めた順）。対戦の ID から部屋を引く（RoomReader.FindByMatch）のに使うため、順を変えない。
	// 空のまま、書くときに repository が公開 ID を入れる
	Matches []uuid.UUID `json:"matches"`
	// 対戦の後の意思表示。人は公開 ID（空のまま、書くときに repository が入れる）
	NextDeclarations []RoomNextDeclarationMetadata `json:"next_declarations"`
	RegisteredAt     time.Time                     `json:"registered_at"`
	UpdatedAt        time.Time                     `json:"updated_at"`
	RegisteredBy     uuid.UUID                     `json:"registered_by"`
	UpdatedBy        uuid.UUID                     `json:"updated_by"`
}

type RoomNextDeclarationMetadata struct {
	Player   uuid.UUID `json:"player"`
	Continue bool      `json:"continue"`
}

// 席 1 つ。座っている人はプレイヤーの席だけ、アバターとデッキはまだ選んでいなければ無い。
// プレイヤーの席のアバターとデッキは avatars・decks、NPC の席は npc_avatars・npc_decks の公開 ID（席の種類で見分ける）
type RoomSeatMetadata struct {
	Kind   room.SeatKind `json:"kind"`
	Player *uuid.UUID    `json:"player,omitempty"`
	Avatar *uuid.UUID    `json:"avatar,omitempty"`
	Deck   *uuid.UUID    `json:"deck,omitempty"`
}

// 対戦ルール。ボードは公開 ID、8 つの値は room.MatchRules と同じ名前
type RoomMatchRulesMetadata struct {
	Board           uuid.UUID `json:"board"`
	TargetMagic     int       `json:"target_magic"`
	StartingMagic   int       `json:"starting_magic"`
	LapBonus        int       `json:"lap_bonus"`
	CheckpointBonus int       `json:"checkpoint_bonus"`
	RoundLimit      int       `json:"round_limit"`
	DiceMin         int       `json:"dice_min"`
	DiceMax         int       `json:"dice_max"`
	BaseLandValue   int       `json:"base_land_value"`
}

// 公開 ID に引き直す部分（ID・席・ボード・人）は空のまま返す
func (state Room) Metadata() RoomMetadata {
	metadata := RoomMetadata{
		Name: state.Name.String(), Status: state.Status, Revision: state.Revision, RegisteredAt: state.RegisteredAt, UpdatedAt: state.UpdatedAt,
	}
	if state.MatchRules != nil {
		rules := state.MatchRules
		metadata.MatchRules = &RoomMatchRulesMetadata{
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus,
			CheckpointBonus: rules.CheckpointBonus, RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax, BaseLandValue: rules.BaseLandValue,
		}
	}
	return metadata
}

// 部屋のイベントだけを扱い、ほかは状態をそのまま返す。公開しただけでは状態は変わらない（定員になって準備中に進む）
func (state Room) Evolve(event any) Room {
	switch e := event.(type) {
	case *room.CreatedEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.Room = e.GetMeta().EntityID
		state.Owner = e.Player
		state.Name = e.Name
		state.Status = RoomStatusDraft
		state.Seats = []RoomSeat{}
		state.Ready = []player.ID{}
		state.Matches = []match.ID{}
		state.NextDeclarations = []RoomNextDeclaration{}
		state.RegisteredAt = occurredAt
		state.touch(e.GetMeta(), e.Player)
	case *room.SeatsChangedEvent:
		state.Seats = make([]RoomSeat, len(e.Seats))
		for i, seat := range e.Seats {
			state.Seats[i] = RoomSeat{Kind: seat.Kind, Player: seat.Player}
		}
		state.touch(e.GetMeta(), state.Owner)
	case *room.MatchRulesChangedEvent:
		rules := e.MatchRules
		state.MatchRules = &rules
		state.touch(e.GetMeta(), state.Owner)
	case *room.SeatChoiceChangedEvent:
		if e.Seat.Int() < len(state.Seats) {
			state.Seats = slices.Clone(state.Seats)
			choice := e.Choice
			state.Seats[e.Seat].Choice = &choice
		}
		state.touch(e.GetMeta(), e.ChangedBy)
	case *room.PublishedEvent:
		state.touch(e.GetMeta(), state.Owner)
	case *room.PreparationStartedEvent:
		state.Status = RoomStatusPreparing
		state.touch(e.GetMeta(), state.Owner)
	case *room.ReadyDeclaredEvent:
		state.Ready = append(slices.Clone(state.Ready), e.Player)
		state.touch(e.GetMeta(), e.Player)
	case *room.MatchStartedEvent:
		state.Status = RoomStatusPlaying
		state.Match = e.Match
		state.Matches = append(slices.Clone(state.Matches), e.Match)
		state.touch(e.GetMeta(), state.Owner)
	case *room.MatchFinishedEvent:
		state.Status = RoomStatusDecidingNext
		state.touch(e.GetMeta(), state.Owner)
	case *room.NextDeclaredEvent:
		state.NextDeclarations = append(slices.Clone(state.NextDeclarations), RoomNextDeclaration{Player: e.Player, Continue: e.Continue})
		state.touch(e.GetMeta(), e.Player)
	case *room.PreparationRestartedEvent:
		state.Status = RoomStatusPreparing
		state.Ready = []player.ID{}
		state.NextDeclarations = []RoomNextDeclaration{}
		state.touch(e.GetMeta(), state.Owner)
	case *room.PausedEvent:
		state.Status = RoomStatusPaused
		state.touch(e.GetMeta(), state.Owner)
	case *room.ClosedEvent:
		state.Status = RoomStatusClosed
		state.touch(e.GetMeta(), state.Owner)
	case *room.RenamedEvent:
		state.Name = e.Name
		state.touch(e.GetMeta(), state.Owner)
	}
	return state
}

func (state *Room) touch(meta room.EventMeta, by player.ID) {
	state.Revision = meta.AggregateVersion
	state.UpdatedAt = meta.OccurredAt
	state.UpdatedBy = by
}

// 部屋を消すイベントは無い（閉じた部屋も行を残し、状態を closed にする）
func (state Room) IsTerminal() bool {
	return false
}

package room

import (
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type State interface {
	isRoomState()
}

type UncreatedRoom struct{}

func (UncreatedRoom) isRoomState() {}

// 席 1 つの状態。選択は「まだ決まっていない（nil）／決まった」で持ち、公開前に決めたものは準備中に変えられない（ChoiceLocked）
type SeatState struct {
	Seat         Seat
	Choice       *SeatChoice
	ChoiceLocked bool
}

// Decide が読む値だけを持つ（名前は変えても同じかを見るために持つ。日時・アバターの名前など画面に出す値は read model がイベントから作る）。
// 席の並び・対戦ルールも「まだ決まっていない／決まった」で持つ（席の並びは空、対戦ルールは nil がまだ）
type Room struct {
	ID      ID
	Version int64
	Owner   player.ID
	Name    Name
	Status  Status
	Seats   []SeatState
	// 対戦ルール。公開前に決めたものは準備中に変えられない
	MatchRules       *MatchRules
	MatchRulesLocked bool
	// 準備完了を申告した人
	Ready []player.ID
}

func (Room) isRoomState() {}

func InitialState() State {
	return UncreatedRoom{}
}

// 部屋に入っている人。今は作った人だけだが、ほかの人が参加できるようになったときに、座っている人を足す
func (r Room) HasParticipant(playerID player.ID) bool {
	if playerID == r.Owner {
		return true
	}
	for _, seat := range r.Seats {
		if seat.Seat.Kind == SeatKindPlayer && seat.Seat.Player == playerID {
			return true
		}
	}
	return false
}

func (r Room) isReady(playerID player.ID) bool {
	return slices.Contains(r.Ready, playerID)
}

// その席の条件（アバターとデッキ）を決める担当。プレイヤーの席は座っている人、NPC の席は作った人
func (r Room) responsibleFor(seat Seat) player.ID {
	if seat.Kind == SeatKindPlayer {
		return seat.Player
	}
	return r.Owner
}

// プレイヤーの席がすべて埋まっている（定員）。今は座るのが作った人だけで、席の並びを決めたときに埋まる
func (r Room) playerSeatsFull() bool {
	for _, seat := range r.Seats {
		if seat.Seat.Kind == SeatKindPlayer && seat.Seat.Player == 0 {
			return false
		}
	}
	return len(r.Seats) > 0
}

package room

import (
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
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
	// 始まった対戦（今の対戦。前の対戦は残さない）。始まるまでは 0
	Match match.ID
	// 対戦の後の意思表示（次を決める間だけ。準備中に戻ったら空に戻す）
	NextDeclarations []NextDeclaration
}

// 対戦の後の意思表示 1 つ。続けるなら Continue
type NextDeclaration struct {
	Player   player.ID
	Continue bool
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

// その人の準備完了で、部屋に入っている全員が準備完了になるか（対戦が始まるか）。準備完了ができない人なら false
func (r Room) StartsMatchOnReady(playerID player.ID) bool {
	if r.checkDeclareReady(playerID) != nil {
		return false
	}
	ready := append(slices.Clone(r.Ready), playerID)
	if !slices.Contains(ready, r.Owner) {
		return false
	}
	for _, seat := range r.Seats {
		if seat.Seat.Kind == SeatKindPlayer && !slices.Contains(ready, seat.Seat.Player) {
			return false
		}
	}
	return true
}

// 対戦の後に意思表示をする人。部屋に入っている全員（作った人と、プレイヤーの席に座っている人。重ねて数えない）
func (r Room) participants() []player.ID {
	participants := []player.ID{r.Owner}
	for _, seat := range r.Seats {
		if seat.Seat.Kind == SeatKindPlayer && !slices.Contains(participants, seat.Seat.Player) {
			participants = append(participants, seat.Seat.Player)
		}
	}
	return participants
}

// いま続けると意思表示する人で、残りの全員がそろうか（準備中に戻るか）。誰かが終えれば部屋は閉じるので、
// 意思表示が残っているのは続ける人だけ
func (r Room) allContinueNow() bool {
	return len(r.NextDeclarations)+1 == len(r.participants())
}

func (r Room) hasDeclaredNext(playerID player.ID) bool {
	return slices.ContainsFunc(r.NextDeclarations, func(d NextDeclaration) bool { return d.Player == playerID })
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

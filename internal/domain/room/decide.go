package room

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため。
// 変えても同じになる操作（同じ席の並び・同じ選択・閉じた部屋を閉じる）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 値が使えるか（持っているアバターか・解放したボードか等）は、選ぶ時にコマンドが確かめてから渡す（準備完了の時には確かめ直さない）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	if create, ok := cmd.(*CreateCommand); ok {
		if _, uncreated := state.(UncreatedRoom); !uncreated {
			return nil, ErrAlreadyCreated
		}
		if missing := MissingRequirements(create.Avatars, create.Decks); len(missing) > 0 {
			return nil, &RequirementsNotMetError{Missing: missing}
		}
		return []Event{&CreatedEvent{
			EventMeta: EventMeta{EntityID: create.ID, AggregateVersion: 1, OccurredAt: now},
			Player:    create.Owner,
			Name:      create.Name,
		}}, nil
	}
	current, ok := state.(Room)
	if !ok {
		return nil, ErrInvalidState
	}
	switch c := cmd.(type) {
	case *ChangeSeatsCommand:
		if err := current.checkChangeSeats(c.ChangedBy); err != nil {
			return nil, err
		}
		seats := make([]Seat, len(c.Seats))
		for i, kind := range c.Seats {
			seats[i] = Seat{Kind: kind}
			if kind == SeatKindPlayer {
				seats[i].Player = current.Owner
			}
		}
		if slices.Equal(seats, seatsOf(current.Seats)) {
			return nil, nil
		}
		return []Event{&SeatsChangedEvent{EventMeta: nextMeta(current, now), Seats: seats}}, nil
	case *ChangeMatchRulesCommand:
		if err := current.checkChangeMatchRules(c.ChangedBy); err != nil {
			return nil, err
		}
		if current.MatchRules != nil && *current.MatchRules == c.MatchRules {
			return nil, nil
		}
		return []Event{&MatchRulesChangedEvent{EventMeta: nextMeta(current, now), MatchRules: c.MatchRules}}, nil
	case *ChangeSeatChoiceCommand:
		if len(current.Seats) == 0 {
			return nil, ErrSeatsNotDecided
		}
		if c.Seat.Int() >= len(current.Seats) {
			return nil, ErrSeatNotFound
		}
		seat := current.Seats[c.Seat]
		if c.Choice.Kind() != seat.Seat.Kind {
			return nil, ErrSeatKindMismatch
		}
		if err := current.checkChangeSeatChoice(c.ChangedBy, c.Seat); err != nil {
			return nil, err
		}
		if seat.Choice != nil && seat.Choice.equal(c.Choice) {
			return nil, nil
		}
		return []Event{&SeatChoiceChangedEvent{EventMeta: nextMeta(current, now), Seat: c.Seat, Choice: c.Choice, ChangedBy: c.ChangedBy}}, nil
	case *PublishCommand:
		if err := current.checkPublish(c.PublishedBy); err != nil {
			return nil, err
		}
		events := []Event{&PublishedEvent{EventMeta: nextMeta(current, now)}}
		// 定員なら続けて準備中に進む。今は公開と同時に定員になる（参加の操作を作るときに、定員になった参加でも出す）
		if current.playerSeatsFull() {
			events = append(events, &PreparationStartedEvent{EventMeta: metaAt(current, current.Version+2, now)})
		}
		return events, nil
	case *DeclareReadyCommand:
		if err := current.checkDeclareReady(c.Player); err != nil {
			return nil, err
		}
		events := []Event{&ReadyDeclaredEvent{EventMeta: nextMeta(current, now), Player: c.Player}}
		if current.StartsMatchOnReady(c.Player) {
			if c.Match == 0 {
				return nil, ErrMatchIDMissing
			}
			events = append(events, &MatchStartedEvent{EventMeta: metaAt(current, current.Version+2, now), Match: c.Match})
		}
		return events, nil
	case *FinishMatchCommand:
		if current.Status != StatusPlaying || current.Match != c.Match {
			return nil, nil
		}
		return []Event{&MatchFinishedEvent{EventMeta: nextMeta(current, now), Match: c.Match}}, nil
	case *DeclareNextCommand:
		if err := current.checkDeclareNext(c.Player); err != nil {
			return nil, err
		}
		events := []Event{&NextDeclaredEvent{EventMeta: nextMeta(current, now), Player: c.Player, Continue: c.Continue}}
		switch {
		case !c.Continue:
			events = append(events, &PausedEvent{EventMeta: metaAt(current, current.Version+2, now)})
		case current.allContinueNow():
			events = append(events, &PreparationRestartedEvent{EventMeta: metaAt(current, current.Version+2, now)})
		}
		return events, nil
	case *ResumeCommand:
		if err := current.checkResume(c.ResumedBy); err != nil {
			return nil, err
		}
		return []Event{&PreparationRestartedEvent{EventMeta: nextMeta(current, now)}}, nil
	case *RenameCommand:
		if err := current.checkRename(c.RenamedBy); err != nil {
			return nil, err
		}
		if current.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(current, now), Name: c.Name}}, nil
	case *CloseCommand:
		if err := current.checkClose(c.ClosedBy); err != nil {
			return nil, err
		}
		if current.Status == StatusClosed {
			return nil, nil
		}
		return []Event{&ClosedEvent{EventMeta: nextMeta(current, now)}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 部屋を作るのに足りないもの（アバター → デッキの、先に作る順）。部屋を作る Decide と、画面に「先に作るもの」を出す問い合わせが
// 同じ関数を通り、決まりが 2 か所にならないようにする。空なら作れる
func MissingRequirements(avatars, decks int) []Requirement {
	missing := []Requirement{}
	if avatars < 1 {
		missing = append(missing, RequirementAvatar)
	}
	if decks < 1 {
		missing = append(missing, RequirementDeck)
	}
	return missing
}

// 操作ごとの「その人にできるか」。Decide と Capabilities（見ている人にできる操作）が同じ関数を通り、決まりが 2 か所にならないようにする。
// 変えても同じになる場合（同じ値・閉じた部屋を閉じ直す）の扱いは Decide の側で決める

func (r Room) checkChangeSeats(actor player.ID) error {
	if actor != r.Owner {
		return ErrNotOwner
	}
	if r.Status != StatusDraft {
		return ErrStatusNotAllowed
	}
	return nil
}

func (r Room) checkChangeMatchRules(actor player.ID) error {
	return r.authorizeChange(actor, r.Owner, r.MatchRulesLocked)
}

// 席が無い・席の番号が範囲の外・種類が違う選択は、入力の側の誤りなのでここでは見ない（席が範囲の中にあるときだけ呼ぶ）
func (r Room) checkChangeSeatChoice(actor player.ID, index SeatIndex) error {
	seat := r.Seats[index]
	return r.authorizeChange(actor, r.responsibleFor(seat.Seat), seat.ChoiceLocked)
}

func (r Room) checkPublish(actor player.ID) error {
	if actor != r.Owner {
		return ErrNotOwner
	}
	if r.Status != StatusDraft {
		return ErrStatusNotAllowed
	}
	if len(r.Seats) == 0 {
		return ErrSeatsNotDecided
	}
	return nil
}

func (r Room) checkDeclareReady(actor player.ID) error {
	if r.Status != StatusPreparing {
		return ErrStatusNotAllowed
	}
	if !r.HasParticipant(actor) {
		return ErrNotParticipant
	}
	if r.isReady(actor) {
		return ErrAlreadyReady
	}
	if !r.decidedFor(actor) {
		return ErrConditionsNotDecided
	}
	return nil
}

func (r Room) checkDeclareNext(actor player.ID) error {
	if r.Status != StatusDecidingNext {
		return ErrStatusNotAllowed
	}
	if !r.HasParticipant(actor) {
		return ErrNotParticipant
	}
	if r.hasDeclaredNext(actor) {
		return ErrAlreadyDeclaredNext
	}
	return nil
}

// 再開は作った人だけ（閉じる・対戦ルールを決めるのと同じ人。誰が再開できるかは、人が参加できるようになったときに「終える」の意味と一緒に決め直す）
func (r Room) checkResume(actor player.ID) error {
	if actor != r.Owner {
		return ErrNotOwner
	}
	if r.Status != StatusPaused {
		return ErrStatusNotAllowed
	}
	return nil
}

// 閉じた部屋の名前は変えられない（閉じた部屋を閉じ直すのと違い、成功にしない。閉じた後は何も変えない部屋だから）
func (r Room) checkRename(actor player.ID) error {
	if actor != r.Owner {
		return ErrNotOwner
	}
	if r.Status == StatusClosed {
		return ErrStatusNotAllowed
	}
	return nil
}

// 閉じた部屋を閉じ直すのは成功（イベントは出ない）なので、この関数は閉じた部屋でも通る。対戦中は閉じられない
// （途中でやめる（投了）は対戦の側で決める）
func (r Room) checkClose(actor player.ID) error {
	if actor != r.Owner {
		return ErrNotOwner
	}
	if r.Status == StatusPlaying {
		return ErrStatusNotAllowed
	}
	return nil
}

// 見ている人にできる操作。画面が自分で判断すると決まりが 2 か所になるため、Decide が通る関数をそのまま使う。
// 席ごとの選択は席の並びの順（ChangeSeatChoice）。何も変わらない操作（閉じた部屋を閉じる）は、押す意味が無いのでできないとする。
// まだ作られていない部屋は、すべてできない（部屋に入っていない人は、各 check が担当でも作った人でもないとして断る）
type Capabilities struct {
	ChangeSeats      bool
	ChangeMatchRules bool
	ChangeSeatChoice []bool
	Publish          bool
	DeclareReady     bool
	// 対戦の後に、続ける・終えるを意思表示できるか（どちらを選んでも同じ条件）
	DeclareNext bool
	// 休止中の部屋を再開できるか
	Resume bool
	Close  bool
	Rename bool
}

func CapabilitiesOf(state State, viewer player.ID) Capabilities {
	current, ok := state.(Room)
	if !ok {
		return Capabilities{}
	}
	capabilities := Capabilities{
		ChangeSeats:      current.checkChangeSeats(viewer) == nil,
		ChangeMatchRules: current.checkChangeMatchRules(viewer) == nil,
		ChangeSeatChoice: make([]bool, len(current.Seats)),
		Publish:          current.checkPublish(viewer) == nil,
		DeclareReady:     current.checkDeclareReady(viewer) == nil,
		DeclareNext:      current.checkDeclareNext(viewer) == nil,
		Resume:           current.checkResume(viewer) == nil,
		Close:            current.checkClose(viewer) == nil && current.Status != StatusClosed,
		Rename:           current.checkRename(viewer) == nil,
	}
	for i := range current.Seats {
		capabilities.ChangeSeatChoice[i] = current.checkChangeSeatChoice(viewer, SeatIndex(i)) == nil
	}
	return capabilities
}

// 条件（対戦ルール・席の選択）を変えてよいか。公開前は作った人だけが決められる。準備中は担当の人だけで、公開前に決めたものは変えられず、
// 準備完了を申告した人は自分の担当の分を変えられない（ほかの人の申告には影響されない）
func (r Room) authorizeChange(actor, responsible player.ID, locked bool) error {
	switch r.Status {
	case StatusDraft:
		if actor != r.Owner {
			return ErrNotOwner
		}
		return nil
	case StatusPreparing:
		if actor != responsible {
			return ErrNotResponsible
		}
		if locked {
			return ErrConditionLocked
		}
		if r.isReady(actor) {
			return ErrAlreadyReady
		}
		return nil
	default:
		return ErrStatusNotAllowed
	}
}

// その人の担当の分（自分の席の選択、作った人なら NPC の席の選択と対戦ルール）がすべて決まっている
func (r Room) decidedFor(playerID player.ID) bool {
	for _, seat := range r.Seats {
		if r.responsibleFor(seat.Seat) == playerID && seat.Choice == nil {
			return false
		}
	}
	return playerID != r.Owner || r.MatchRules != nil
}

func seatsOf(states []SeatState) []Seat {
	seats := make([]Seat, len(states))
	for i, state := range states {
		seats[i] = state.Seat
	}
	return seats
}

func nextMeta(current Room, now time.Time) EventMeta {
	return metaAt(current, current.Version+1, now)
}

func metaAt(current Room, version int64, now time.Time) EventMeta {
	return EventMeta{EntityID: current.ID, AggregateVersion: version, OccurredAt: now}
}

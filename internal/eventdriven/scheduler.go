package eventdriven

import (
	"sync"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
)

// 1 つの read model の行（購読者 × 集約）
type projectionKey struct {
	subscriberID int16
	entityTypeID domaindef.EntityTypeID
	entityID     int64
}

type keyState struct {
	running bool
	// 処理中に、また知らせが来た。処理中に読んだイベントには新しい版が入っていないかもしれないため、終わった後にもう 1 回作り直す
	dirty bool
}

// 作り直しを待つキーの列。投影は毎回すべてのイベントを読み直すため、同じキーの知らせはまとめ、
// 待っている間に何回来ても作り直しは 1 回にする（参照の api はキーごとにタスクの FIFO を持ち、同じ結果を何度も書く）。
// 列に上限を設けず知らせを捨てないのは、キーごとにまとめるため列の長さが待っている集約の数を超えないから
// （参照の api は満杯なら捨てて Poller に任せるが、Poller の周期まで遅れる）。
// ポリシーはイベントを 1 件ずつ順に処理するため、この作りは使えない
type scheduler struct {
	mu     sync.Mutex
	cond   *sync.Cond
	states map[projectionKey]*keyState
	ready  []projectionKey
	closed bool
}

func newScheduler() *scheduler {
	s := &scheduler{states: make(map[projectionKey]*keyState)}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// 待たずに返す（知らせを受け取る Listener を止めないため）
func (s *scheduler) enqueue(key projectionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[key]
	if !ok {
		s.states[key] = &keyState{}
		s.push(key)
		return
	}
	if state.running {
		state.dirty = true
	}
}

// 作り直すキーを待って返す。close の後は false を返す
func (s *scheduler) next() (projectionKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.ready) == 0 && !s.closed {
		s.cond.Wait()
	}
	if s.closed {
		return projectionKey{}, false
	}
	key := s.ready[0]
	s.ready = s.ready[1:]
	s.states[key].running = true
	return key, true
}

// next で受け取ったキーの処理を終えたら、成否によらず必ず呼ぶ
func (s *scheduler) done(key projectionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	if !state.dirty {
		delete(s.states, key)
		return
	}
	state.running = false
	state.dirty = false
	s.push(key)
}

func (s *scheduler) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.cond.Broadcast()
}

// mu を持って呼ぶ
func (s *scheduler) push(key projectionKey) {
	s.ready = append(s.ready, key)
	s.cond.Signal()
}

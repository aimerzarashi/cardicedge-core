package eventdriven

import (
	"sync"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
)

// 1 つの処理の単位（購読者 × 集約）。購読者の鍵 subscriberKey（kind, name）と紛れないよう work と呼ぶ
type workKey struct {
	subscriberID int16
	entityTypeID domaindef.EntityTypeID
	entityID     int64
}

type keyState struct {
	running bool
	// 処理中に、また知らせが来た。処理中に読んだイベントには新しい版が入っていないかもしれないため、終わった後にもう 1 回作り直す
	dirty bool
}

// 処理を待つキーの列。同じキーの知らせはまとめ、待っている間に何回来ても処理は 1 回にする
// （参照の api はキーごとにタスクの FIFO を持ち、同じ結果を何度も書く）。まとめてよいのは、投影もポリシーも、処理のたびに
// 集約の全イベントを読み、投影は最後の状態を作り直し、ポリシーはチェックポイントより後のイベントを版の順に全部こなすため、
// 知らせの数によらず取りこぼさないから。
// 列に上限を設けず知らせを捨てないのは、キーごとにまとめるため列の長さが待っている集約の数を超えないから
// （参照の api は満杯なら捨てて Poller に任せるが、Poller の周期まで遅れる）
type scheduler struct {
	mu     sync.Mutex
	cond   *sync.Cond
	states map[workKey]*keyState
	ready  []workKey
	closed bool
}

func newScheduler() *scheduler {
	s := &scheduler{states: make(map[workKey]*keyState)}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// 待たずに返す（知らせを受け取る Listener を止めないため）
func (s *scheduler) enqueue(key workKey) {
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
func (s *scheduler) next() (workKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.ready) == 0 && !s.closed {
		s.cond.Wait()
	}
	if s.closed {
		return workKey{}, false
	}
	key := s.ready[0]
	s.ready = s.ready[1:]
	s.states[key].running = true
	return key, true
}

// next で受け取ったキーの処理を終えたら、成否によらず必ず呼ぶ
func (s *scheduler) done(key workKey) {
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

// next で受け取ったキーを、処理せずに手放す（ほかの台が処理中で、ロックが取れなかったとき。core#19）。処理中に来た知らせの印も
// 捨てる。その知らせのイベントは、ロックを持つ台が処理中に知らせを受けてやり直すか、やることの表に残って Poller が拾うため
func (s *scheduler) skip(key workKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, key)
}

func (s *scheduler) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.cond.Broadcast()
}

// mu を持って呼ぶ
func (s *scheduler) push(key workKey) {
	s.ready = append(s.ready, key)
	s.cond.Signal()
}

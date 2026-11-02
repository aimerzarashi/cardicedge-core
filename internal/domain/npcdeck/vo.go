package npcdeck

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("npc_deck")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("npc deck id must be positive")
	}
	return ID(value), nil
}

// カードの状態（card.Status）と移り方は同じだが、型は共有しない。 NPC のデッキの状態の決まりをカードの集約の
// 状態と一緒に変えることになり、片方だけ移り方を変えたくなったときに分けられなくなるため（card パッケージに依存もしない）。
// 準備中は名前・カードを自由に変えられ、決まり（合計の枚数等）を満たさなくてよい。公開中は決まりを満たす。
// 公開中 → 準備中に戻せないのは、公開した中身が途中で変わるため
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

// DB の npc_decks.status など、外から来た値を状態にする
func NewStatus(value string) (Status, error) {
	status := Status(value)
	switch status {
	case StatusDraft, StatusPublished, StatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrStatusUnknown, value)
	}
}

func (s Status) String() string {
	return string(s)
}

// 公開した後の状態と、状態が変わるか。準備中・使用不可からは公開中にでき、公開中にもう一度公開しても変わらない
func (s Status) Publish() (Status, bool, error) {
	switch s {
	case StatusDraft, StatusRetired:
		return StatusPublished, true, nil
	case StatusPublished:
		return StatusPublished, false, nil
	default:
		return s, false, ErrStatusUnknown
	}
}

// 使用不可にした後の状態と、状態が変わるか。一度も公開していない準備中は使用不可にできない。使用不可にもう一度しても変わらない
func (s Status) Retire() (Status, bool, error) {
	switch s {
	case StatusPublished:
		return StatusRetired, true, nil
	case StatusRetired:
		return StatusRetired, false, nil
	case StatusDraft:
		return s, false, ErrInvalidStatusTransition
	default:
		return s, false, ErrStatusUnknown
	}
}

// どのプリセットデッキの、どの時点から写したか。元のプリセットデッキは消してよい（参照にしない）ため、
// 消えた後も読めるよう、内部 ID ではなく公開 ID で持つ
type Source struct {
	presetDeckID uuid.UUID
	revision     int64
}

func NewSource(presetDeckID uuid.UUID, revision int64) (Source, error) {
	if presetDeckID == uuid.Nil || revision <= 0 {
		return Source{}, errors.New("npcdeck: source needs a preset deck id and a positive revision")
	}
	return Source{presetDeckID: presetDeckID, revision: revision}, nil
}

func (s Source) PresetDeckID() uuid.UUID {
	return s.presetDeckID
}

func (s Source) Revision() int64 {
	return s.revision
}

type sourceJSON struct {
	PresetDeckID uuid.UUID `json:"preset_deck_id"`
	Revision     int64     `json:"revision"`
}

func (s Source) MarshalJSON() ([]byte, error) {
	return json.Marshal(sourceJSON{PresetDeckID: s.presetDeckID, Revision: s.revision})
}

func (s *Source) UnmarshalJSON(data []byte) error {
	var v sourceJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = Source{presetDeckID: v.PresetDeckID, revision: v.Revision}
	return nil
}

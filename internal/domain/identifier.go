package domain

import (
	"context"

	"github.com/google/uuid"
)

type ID interface {
	~int64
	Int64() int64
	// 種別名をパッケージ名からリフレクションで取り出さず宣言させるのは、パッケージの名前を変えただけで
	// domain_entity_types と食い違うのを防ぐため。基底型がたまたま int64 の型を ID として渡せないようにする役目も兼ねる
	EntityType() EntityType
}

// 集約の種別名（domain_entity_types の名前）。各集約だけが生成関数で宣言する。
// ほかの名前（イベントの種別名・一意な key の種別）と同じ string にしないのは、取り違えてもコンパイルが通ってしまうため
type EntityType string

func NewEntityType(name string) EntityType { return EntityType(name) }

func (t EntityType) String() string { return string(t) }

type NewIDFunc[I ID] func(int64) (I, error)

type Identifier[I ID] interface {
	Issue(context.Context) (I, uuid.UUID, error)
	Resolve(context.Context, uuid.UUID) (I, error)
}

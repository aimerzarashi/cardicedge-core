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
	EntityType() string
}

type NewIDFunc[I ID] func(int64) (I, error)

type Identifier[I ID] interface {
	Issue(context.Context) (I, uuid.UUID, error)
	Resolve(context.Context, uuid.UUID) (I, error)
}

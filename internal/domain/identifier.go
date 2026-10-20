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

func NewEntityType(name string) EntityType {
	return EntityType(name)
}

func (t EntityType) String() string {
	return string(t)
}

type NewIDFunc[I ID] func(int64) (I, error)

type Identifier[I ID] interface {
	Issue(context.Context) (I, uuid.UUID, error)
	// 無ければ ErrEntityNotFound
	Resolve(context.Context, uuid.UUID) (I, error)
	// 一意な key（domain_unique_keys）は外部キーのカスケードで一緒に消え、ほかの集約が使えるようになる（key の種別ごとに
	// 手放す関数を持つと、key を足したときに手放し忘れが起きうるため）。イベントは残す。参照の api のアバターの削除と同じ
	Delete(context.Context, I) error
}

// 種類をまたいでほかの集約を指すときの、種類と内部 ID の組（例: プリセットデッキに入れるカード）。
// 種類は DB の entity_type_id ではなく種別名で持つ（ドメインは種別を名前で扱うため）
type EntityRef struct {
	entityType EntityType
	id         int64
}

func NewEntityRef(entityType EntityType, id int64) EntityRef {
	return EntityRef{entityType: entityType, id: id}
}

func (r EntityRef) EntityType() EntityType {
	return r.entityType
}

func (r EntityRef) ID() int64 {
	return r.id
}

// 種類を決めずに、公開 ID と種類・内部 ID の組を行き来する（Identifier は種類を 1 つ決めて引くため、どの種類か
// わからない公開 ID を引けない）。何の種類なら受け付けるかは、呼ぶ側（例: presetdeck.NewCardRef）が決める
type EntityRefResolver interface {
	// 無ければ ErrEntityNotFound
	Resolve(context.Context, uuid.UUID) (EntityRef, error)
	// まとめて引く（1 回の読み出し）。無いものは結果に含めない
	ResolveAll(context.Context, []uuid.UUID) (map[uuid.UUID]EntityRef, error)
	// 無いものは結果に含めない
	PublicIDs(context.Context, []EntityRef) (map[EntityRef]uuid.UUID, error)
}

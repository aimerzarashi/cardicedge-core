package domain

// 一意な key の種別（domain_unique_key_types の名前。何を一意にするか）。決まりの持ち主（各集約・カード）だけが生成関数で宣言する。
// 値と同じ string にしないのは、NewUniqueKey の引数を取り違えても（種別と値の入れ替え）コンパイルが通ってしまうため
type UniqueKeyType string

func NewUniqueKeyType(name string) UniqueKeyType {
	return UniqueKeyType(name)
}

func (t UniqueKeyType) String() string {
	return string(t)
}

// 一意性の制約の key（種別と値の組）。作り方（正規化、複数の値のつなぎ方）は決まりの持ち主の生成関数だけが決め、
// インフラは値の意味を知らずに「種別の中で一意に確保する」だけにする（種別と値の組み合わせ・正規化の取り違えを、作る 1 か所に閉じるため）
type UniqueKey struct {
	keyType UniqueKeyType
	value   string
}

func NewUniqueKey(keyType UniqueKeyType, value string) UniqueKey {
	return UniqueKey{keyType: keyType, value: value}
}

func (k UniqueKey) Type() UniqueKeyType {
	return k.keyType
}

func (k UniqueKey) Value() string {
	return k.value
}

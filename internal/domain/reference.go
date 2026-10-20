package domain

// 参照の種別（domain_reference_types の名前。何が何を参照するか）。参照する側の集約だけが生成関数で宣言する。
// ほかの名前と同じ string にしない理由は UniqueKeyType と同じ
type ReferenceType string

func NewReferenceType(name string) ReferenceType {
	return ReferenceType(name)
}

func (t ReferenceType) String() string {
	return string(t)
}

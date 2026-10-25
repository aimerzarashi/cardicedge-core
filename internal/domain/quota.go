package domain

import "errors"

// 上限の種別（domain_quota_types の名前。何の数を数えるか）。数える集約だけが生成関数で宣言する。
// ほかの名前と同じ string にしない理由は UniqueKeyType と同じ
type QuotaType string

func NewQuotaType(name string) QuotaType {
	return QuotaType(name)
}

func (t QuotaType) String() string {
	return string(t)
}

// 上限の対象（種別と、数える単位の持ち主の集約の内部 ID の組）。持ち主ごとの単位の作り方は、数える集約の生成関数だけが決める
// （UniqueKey と同じく、種別と持ち主の取り違えを作る 1 か所に閉じるため）
type Quota struct {
	quotaType QuotaType
	ownerID   int64
}

func NewQuota(quotaType QuotaType, ownerID int64) Quota {
	return Quota{quotaType: quotaType, ownerID: ownerID}
}

func (q Quota) Type() QuotaType {
	return q.quotaType
}

func (q Quota) OwnerID() int64 {
	return q.ownerID
}

var ErrInvalidQuotaLimit = errors.New("domain: quota limit must be at least 1")

// 上限の値（1 以上）。0 以下は「何も作れない」か誤りであり、確保の文に渡す前に弾く。
// 型付きの定数で既定の値を書けるよう、int を元にする
type QuotaLimit int

func NewQuotaLimit(value int) (QuotaLimit, error) {
	if value < 1 {
		return 0, ErrInvalidQuotaLimit
	}
	return QuotaLimit(value), nil
}

func (l QuotaLimit) Int() int {
	return int(l)
}

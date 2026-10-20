package domain

import "errors"

var (
	ErrUnknownEventType = errors.New("unknown event type")
	ErrVersionConflict  = errors.New("aggregate version conflict")
	// 版が続いていないイベントを保存しようとした（版 1 でも直前の版の次でもない）。操作が重なって起きる衝突
	// （ErrVersionConflict）と違い、版の作り方の誤り（コードの誤り）でだけ起きるので、呼ぶ側は分岐せずにそのまま返す
	ErrVersionNotConsecutive = errors.New("aggregate version not consecutive")
	// 公開 ID・内部 ID に当たる識別子が無い（発行していない、または削除した）
	ErrEntityNotFound = errors.New("entity not found")
)

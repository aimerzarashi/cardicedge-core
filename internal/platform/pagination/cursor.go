package pagination

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidCursor = errors.New("pagination: invalid cursor")

// カーソルの中身の型が満たす。JSON は足りない項目をゼロ値のまま通すため、必要な値がそろっているかを型ごとに確かめる
type Cursor interface {
	Validate() error
}

// base64 は URL でそのまま使える形にする（cms が URL のクエリにカーソルを載せても、エスケープが要らないように）
func EncodeCursor[C Cursor](cursor C) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("pagination: encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func DecodeCursor[C Cursor](encoded string) (C, error) {
	var cursor C
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return cursor, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return cursor, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	// 1 つ目の値の後ろに続きがあれば、作ったカーソルではない
	if decoder.More() {
		return cursor, fmt.Errorf("%w: trailing data", ErrInvalidCursor)
	}
	if err := cursor.Validate(); err != nil {
		return cursor, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	return cursor, nil
}

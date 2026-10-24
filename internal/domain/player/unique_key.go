package player

import (
	"context"
	"fmt"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_unique_key_types の名前。起動時に照合する
var SubjectKeyType = domain.NewUniqueKeyType("player.subject")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{SubjectKeyType}
}

// iss の文字数を前に付けてつなぐのは、区切り文字でつなぐと、iss・sub に区切り文字が入ったときに別の組と同じ key になりうるため
func SubjectKey(issuer Issuer, subject Subject) domain.UniqueKey {
	return domain.NewUniqueKey(SubjectKeyType, fmt.Sprintf("%d:%s%s", utf8.RuneCountInString(issuer.String()), issuer, subject))
}

// ほかのプレイヤーがその (iss, sub) で登録していれば ErrAlreadyRegistered
type UpdateSubjectFunc func(ctx context.Context, id ID, issuer Issuer, subject Subject) error

// 一意性の key から、確保しているプレイヤーを引く（確保する関数と同じく、実装は internal/constraint）
type FindBySubjectFunc func(ctx context.Context, issuer Issuer, subject Subject) (ID, bool, error)

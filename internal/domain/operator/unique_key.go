package operator

import (
	"context"
	"fmt"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_unique_key_types の名前。起動時に照合する。インフラが種別をまたいで引く（未紐づけのオペレーター）ときにも使う
var (
	EmailKeyType   = domain.NewUniqueKeyType("operator.email")
	SubjectKeyType = domain.NewUniqueKeyType("operator.subject")
)

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{EmailKeyType, SubjectKeyType}
}

func EmailKey(email Email) domain.UniqueKey {
	return domain.NewUniqueKey(EmailKeyType, email.String())
}

// iss の文字数を前に付けてつなぐのは、区切り文字でつなぐと、iss・sub に区切り文字が入ったときに別の組と同じ key になりうるため
func SubjectKey(issuer Issuer, subject Subject) domain.UniqueKey {
	return domain.NewUniqueKey(SubjectKeyType, fmt.Sprintf("%d:%s%s", utf8.RuneCountInString(issuer.String()), issuer, subject))
}

// 登録にも変更にも使う。ほかのオペレーターがそのメールアドレスを使っていれば ErrAlreadyRegistered
type UpdateEmailFunc func(ctx context.Context, id ID, email Email) error

// ほかのオペレーターがその (iss, sub) に紐づいていれば ErrAlreadyLinked。紐づけ直しを断るのは Decide の役目
type UpdateSubjectFunc func(ctx context.Context, id ID, issuer Issuer, subject Subject) error

// 一意性の key から、確保しているオペレーターを引く（確保する関数と同じく、実装は internal/constraint）
type (
	FindBySubjectFunc func(ctx context.Context, issuer Issuer, subject Subject) (ID, bool, error)
	// 紐づけの有無を問わず、メールアドレスで引く
	FindByEmailFunc func(ctx context.Context, email Email) (ID, bool, error)
	// 紐づけていない（(iss, sub) の key を持たない）オペレーターだけを返す
	FindUnlinkedByEmailFunc func(ctx context.Context, email Email) (ID, bool, error)
	ExistsFunc              func(ctx context.Context) (bool, error)
)

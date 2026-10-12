package operator

import (
	"errors"
	"net/mail"
	"strings"
)

type ID int64

const entityType = "operator"

func (ID) EntityType() string { return entityType }

func (id ID) Int64() int64 { return int64(id) }

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("operator id must be positive")
	}
	return ID(value), nil
}

type Issuer string

func NewIssuer(value string) (Issuer, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("issuer must not be empty")
	}
	return Issuer(value), nil
}

func (i Issuer) String() string { return string(i) }

// 中身を解釈しない（better-auth の内部 ID は外部の識別子としてだけ扱う方針）
type Subject string

func NewSubject(value string) (Subject, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("subject must not be empty")
	}
	return Subject(value), nil
}

func (s Subject) String() string { return string(s) }

// better-auth と同じく小文字にそろえる（JWT のメールアドレスと照合するため）
type Email string

func NewEmail(value string) (Email, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	// ParseAddress は「名前 <アドレス>」の形も受け付けるため、アドレスだけの形かも確かめる
	addr, err := mail.ParseAddress(normalized)
	if err != nil || addr.Address != normalized {
		return "", errors.New("email is invalid")
	}
	return Email(normalized), nil
}

func (e Email) String() string { return string(e) }

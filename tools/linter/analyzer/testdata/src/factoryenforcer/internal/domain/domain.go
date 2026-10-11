package domain

import "errors"

// Code はVOの例（factory_enforcer のテスト用）
type Code string

func NewCode(v string) (Code, error) {
	if v == "" {
		return "", errors.New("code must not be empty")
	}
	return Code(v), nil
}

// Status はVOの例
type Status string

func NewStatus(v string) (Status, error) {
	if v == "" {
		return "", errors.New("status must not be empty")
	}
	return Status(v), nil
}

// State はVO型フィールドを持つ構造体
type State struct {
	Code   Code
	Status Status
}

func NewState(code Code, status Status) State {
	return State{Code: code, Status: status}
}

// PlainState はプリミティブ型フィールドのみを持つ構造体（VOではない）
type PlainState struct {
	Code   string
	Status string
}

// Email は型変換で生成するVOの例
type Email string

func NewEmail(v string) (Email, error) {
	if v == "" {
		return "", errors.New("email must not be empty")
	}
	return Email(v), nil
}

// Profile はネストしたVO埋め込みのテスト用
type Profile struct {
	Name string
}

func NewProfile(name string) Profile {
	return Profile{Name: name}
}

// User はProfile（対象レイヤー型）をフィールドに持つ集約
type User struct {
	Profile Profile
}

func NewUser(profile Profile) User {
	return User{Profile: profile}
}

// PermissionRow はプリミティブ型フィールドのみを持つ ReadModel の例
// （スライスリテラルの要素として直接生成しても違反にならないことの回帰テスト用）
type PermissionRow struct {
	Resource string
	Action   string
}

// Ticket はポインタ型のVOフィールドを持つ構造体（ポインタ剥がしロジックのテスト用）
type Ticket struct {
	State *State
}

func NewTicket(state *State) Ticket {
	return Ticket{State: state}
}

// MustNewCode はエラーを返さないCode生成ヘルパー（テスト用）
func MustNewCode(v string) Code {
	c, err := NewCode(v)
	if err != nil {
		panic(err)
	}
	return c
}

// MustNewStatus はエラーを返さないStatus生成ヘルパー（テスト用）
func MustNewStatus(v string) Status {
	s, err := NewStatus(v)
	if err != nil {
		panic(err)
	}
	return s
}

// Repository はDI パターンのテスト用インターフェース
// ゼロ値（nil）は正当な初期状態で、後から実装を注入するため、定義型のゼロ値宣言検知対象外
type Repository interface {
	Find(id string) error
}

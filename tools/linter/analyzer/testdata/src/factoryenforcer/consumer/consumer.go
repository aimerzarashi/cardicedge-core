package consumer

import "factoryenforcer/internal/domain"

// 型変換の直接生成: NG
func directConversion() domain.Email {
	return domain.Email("a@example.com") // want `direct type conversion of factoryenforcer/internal/domain\.Email is prohibited\. Use a factory function\.`
}

// スライス要素での型変換: NG
func directConversionInSlice() []domain.Email {
	return []domain.Email{
		domain.Email("a@example.com"), // want `direct type conversion of factoryenforcer/internal/domain\.Email is prohibited\. Use a factory function\.`
	}
}

// ファクトリー経由の生成: OK
func viaFactory() domain.Email {
	email, err := domain.NewEmail("a@example.com")
	if err != nil {
		return ""
	}
	return email
}

// VO型フィールドへの直接値埋め込み（keyed）: NG
func directFieldAssignmentKeyed() domain.State {
	return domain.State{
		Code:   "STAFF001", // want `direct assignment of factoryenforcer/internal/domain\.Code is prohibited\. Pass a pre-validated variable\.`
		Status: "ACTIVE",   // want `direct assignment of factoryenforcer/internal/domain\.Status is prohibited\. Pass a pre-validated variable\.`
	}
}

// VO型フィールドへの直接値埋め込み（unkeyed）: NG
func directFieldAssignmentUnkeyed() domain.State {
	return domain.State{
		"STAFF001", // want `direct assignment of factoryenforcer/internal/domain\.Code is prohibited\. Pass a pre-validated variable\.`
		"ACTIVE",   // want `direct assignment of factoryenforcer/internal/domain\.Status is prohibited\. Pass a pre-validated variable\.`
	}
}

// プリミティブ型フィールドのみの構造体: OK（VOの埋め込みが存在しないため）
func plainFieldsAllowed() domain.PlainState {
	return domain.PlainState{Code: "STAFF001", Status: "ACTIVE"}
}

// ファクトリー生成済みVOを構造体に埋め込む: OK
func preValidatedFieldAssignment() domain.State {
	code, err := domain.NewCode("STAFF001")
	if err != nil {
		return domain.State{}
	}
	status, err := domain.NewStatus("ACTIVE")
	if err != nil {
		return domain.State{}
	}
	return domain.State{Code: code, Status: status}
}

// ネストした構造体リテラルの直接生成: NG
func nestedRawLiteral() domain.User {
	return domain.User{
		Profile: domain.Profile{Name: "Alice"}, // want `direct assignment of factoryenforcer/internal/domain\.Profile is prohibited\. Pass a pre-validated variable\.`
	}
}

// ネストした構造体リテラルの中身が変数のみ: NG（型自体の直接生成として検知）
func nestedLiteralWithVariable(name string) domain.User {
	return domain.User{
		Profile: domain.Profile{Name: name}, // want `direct assignment of factoryenforcer/internal/domain\.Profile is prohibited\. Pass a pre-validated variable\.`
	}
}

// ネストした空の構造体リテラル: NG
func nestedEmptyLiteral() domain.User {
	return domain.User{
		Profile: domain.Profile{}, // want `direct assignment of factoryenforcer/internal/domain\.Profile is prohibited\. Pass a pre-validated variable\.`
	}
}

// ファクトリー生成済みVOをネストしたフィールドに埋め込む: OK
func preValidatedNestedAssignment() domain.User {
	profile := domain.NewProfile("Alice")
	return domain.User{Profile: profile}
}

// プリミティブ型フィールドのみの構造体をスライス要素として直接生成: OK
// （operatorrole_list_test.go で発生した回帰: スライス要素を構造体フィールドと誤認しないことの確認）
func sliceOfPlainStructLiteralsKeyed() []domain.PermissionRow {
	return []domain.PermissionRow{
		{Resource: "card", Action: "read"},
		{Resource: "card", Action: "write"},
	}
}

// プリミティブ型フィールドのみの構造体をスライス要素として unkeyed で直接生成: OK
func sliceOfPlainStructLiteralsUnkeyed() []domain.PermissionRow {
	return []domain.PermissionRow{
		{"card", "read"},
	}
}

// 関数の引数に直接型変換を渡す: NG
func acceptEmail(email domain.Email) {
	_ = email
}

func directConversionAsArgument() {
	acceptEmail(domain.Email("a@example.com")) // want `direct type conversion of factoryenforcer/internal/domain\.Email is prohibited\. Use a factory function\.`
}

// マップリテラルの値としての直接型変換: NG
func directConversionInMap() map[string]domain.Email {
	return map[string]domain.Email{
		"primary": domain.Email("a@example.com"), // want `direct type conversion of factoryenforcer/internal/domain\.Email is prohibited\. Use a factory function\.`
	}
}

// インデックス代入での直接型変換: NG
func directConversionInIndexAssignment() {
	emails := make([]domain.Email, 1)
	emails[0] = domain.Email("a@example.com") // want `direct type conversion of factoryenforcer/internal/domain\.Email is prohibited\. Use a factory function\.`
}

// ポインタ型VOフィールドへのリテラル直接生成（&付き）: NG
// （&CompositeLit も ast.Inspect が独立して内側の State{...} を再訪するため、
//
//	ネストした各フィールドの直接値埋め込みとして検知される）
func directPointerFieldAssignment() domain.Ticket {
	return domain.Ticket{
		State: &domain.State{
			Code:   "STAFF001", // want `direct assignment of factoryenforcer/internal/domain\.Code is prohibited\. Pass a pre-validated variable\.`
			Status: "ACTIVE",   // want `direct assignment of factoryenforcer/internal/domain\.Status is prohibited\. Pass a pre-validated variable\.`
		},
	}
}

// ファクトリー生成済みVOのポインタをフィールドに埋め込む: OK
func preValidatedPointerFieldAssignment() (domain.Ticket, error) {
	code, err := domain.NewCode("STAFF001")
	if err != nil {
		return domain.Ticket{}, err
	}
	status, err := domain.NewStatus("ACTIVE")
	if err != nil {
		return domain.Ticket{}, err
	}
	state := domain.NewState(code, status)
	return domain.Ticket{State: &state}, nil
}

// 構造体型のゼロ値宣言: OK（defined typeではないため対象外、デシリアライズパターンで許可）
func zeroValueStructDeclarationAllowed() domain.State {
	var state domain.State
	return state
}

// 複数変数のゼロ値宣言: NG（同一型の複数変数はそれぞれ検知される）
func zeroValueMultipleDeclarationNG() {
	var code, status domain.Code // want `zero-value variable declaration of factoryenforcer/internal/domain\.Code is prohibited\. Use a factory function\.` `zero-value variable declaration of factoryenforcer/internal/domain\.Code is prohibited\. Use a factory function\.`
	_ = code
	_ = status
}

// プリミティブ型のゼロ値宣言: OK（VO でないため対象外）
func zeroValuePrimitiveAllowed() {
	var count int
	_ = count
}

// PlainState（VO でない構造体）のゼロ値宣言: OK（VO でないため対象外）
func zeroValuePlainStructAllowed() domain.PlainState {
	var state domain.PlainState
	return state
}

// ゼロ値の後に条件付き代入: NG（代入先や用途を問わず、ゼロ値var宣言自体が禁止）
func zeroValueWithConditionalAssignmentNG() {
	var code domain.Code // want `zero-value variable declaration of factoryenforcer/internal/domain\.Code is prohibited\. Use a factory function\.`
	if c, err := domain.NewCode("STAFF001"); err == nil {
		code = c
	}
	_ = code
}

// 初期化子付きのvar宣言（宣言時に値を代入）: OK（バリデーションが宣言時に完了）
func zeroValueWithInitializerAllowed() domain.Code {
	var code domain.Code = domain.MustNewCode("STAFF001")
	return code
}

// ポインタ型VOフィールドを持つ構造体のゼロ値宣言: OK（構造体型のため対象外）
// （Ticket は State フィールドを持つが、フィールドの型は構造体のため defined type ではない）
func zeroValueStructWithPointerVOFieldAllowed() domain.Ticket {
	var ticket domain.Ticket
	return ticket
}

// インターフェース型のゼロ値宣言: OK（DIパターンで実装を後から注入するため）
// ゼロ値（nil）は正当な初期状態
func zeroValueInterfaceDeclarationAllowed() domain.Repository {
	var repo domain.Repository
	return repo
}

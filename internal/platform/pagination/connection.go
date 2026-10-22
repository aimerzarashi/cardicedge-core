package pagination

// GraphQL の型（presentation/graphql/model）に頼らないため、自分の型で返す。GraphQL の型には presentation 層で変える
type Connection[T any] struct {
	Edges    []Edge[T]
	PageInfo PageInfo
}

type Edge[T any] struct {
	Node   T
	Cursor string
}

type PageInfo struct {
	HasNextPage bool
	// 前向きだけなので、前のページがあるかを DB に問わず、after を渡されたかで決める（Relay の仕様は、効率よく
	// 分からなければ false でよいとする。studio が「最初のページへ戻る」を出す手がかりにするため、false 固定にはしない）
	HasPreviousPage bool
	StartCursor     *string
	EndCursor       *string
}

// rows は condition.FetchLimit() 件まで取った行。cursorOf は行からカーソルの中身を作る
func BuildConnection[T any, C Cursor](rows []T, condition Condition[C], cursorOf func(T) C) (Connection[T], error) {
	connection := Connection[T]{
		Edges: []Edge[T]{},
		PageInfo: PageInfo{
			HasNextPage:     len(rows) > condition.First,
			HasPreviousPage: condition.After != nil,
		},
	}
	if connection.PageInfo.HasNextPage {
		rows = rows[:condition.First]
	}
	for _, row := range rows {
		cursor, err := EncodeCursor(cursorOf(row))
		if err != nil {
			return Connection[T]{}, err
		}
		connection.Edges = append(connection.Edges, Edge[T]{Node: row, Cursor: cursor})
	}
	if len(connection.Edges) > 0 {
		start, end := connection.Edges[0].Cursor, connection.Edges[len(connection.Edges)-1].Cursor
		connection.PageInfo.StartCursor = &start
		connection.PageInfo.EndCursor = &end
	}
	return connection, nil
}

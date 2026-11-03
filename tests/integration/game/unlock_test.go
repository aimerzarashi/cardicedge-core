package game_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const meBoardsQuery = `{
	me {
		... on Player { boards { ` + boardFields + ` } }
	}
}`

func meBoards(t *testing.T, token string) ([]any, error) {
	t.Helper()
	result, err := client.TryData(t.Context(), token, meBoardsQuery, nil, "me")
	if err != nil {
		return nil, err
	}
	return result["boards"].([]any), nil
}

// 解放したボードは、スターターセットが指す順（公開中B → 公開中A）に並ぶ。セットは使用不可のボードも指すが、公開中でないものは出ない。
// 準備中のボードと同じく、使用不可のボードも Query.boards には出ない
func TestMe_ListsUnlockedPublishedBoardsInUnlockOrderAfterStarterGrant(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	registerPlayer(t, token)

	// 解放がまだ届いていなくても、エラーにせず空の一覧を返す（null にしない）
	_, err := meBoards(t, token)
	require.NoError(t, err)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		boards, err := meBoards(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, boards, 2) {
			return
		}
		assert.Equal(c, publishedBoardB(), boards[0])
		assert.Equal(c, publishedBoardA(), boards[1])
	})
}

// game 用 GraphQL の結合テスト。本番と同じ組み立て（internal/wiring）でアプリを動かし、HTTP で呼ぶ
package game_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

var (
	endpoint string
	client   testhelper.GraphQLClient
	key      idptest.Key
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// studio の結合テストと同じく、組み立てたアプリを 1 つ作り、並列のテストで共有する
func run(m *testing.M) int {
	if os.Getenv("TEST_DB_DSN") == "" {
		fmt.Fprintln(os.Stderr, "TEST_DB_DSN is not set. run via 'make test'")
		return 0
	}
	errorLogs := testhelper.RecordErrorLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, err := testhelper.StartApp(ctx)
	if err != nil {
		return fail(err)
	}
	defer app.Close()
	key = app.Key
	endpoint = app.URL + "/game/graphql"
	client = testhelper.GraphQLClient{URL: endpoint}

	code := m.Run()

	app.Close()
	if records := errorLogs.Records(); len(records) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: アプリが ERROR のログを %d 件出した\n", len(records))
		for _, r := range records {
			fmt.Fprintln(os.Stderr, "  "+r)
		}
		return 1
	}
	return code
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "FAIL: 結合テストの準備に失敗した:", err)
	return 1
}

// studio 用 GraphQL の結合テスト。本番と同じ組み立て（internal/wiring）でアプリを動かし、HTTP で呼ぶ
package studio_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

var (
	endpoint  string
	client    testhelper.GraphQLClient
	key       idptest.Key
	operators *testhelper.Operators
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// 複製した DB と組み立てたアプリを 1 つずつ作り、並列のテストで共有する（テストごとに作ると、Hub・Listener が
// テストの数だけ動くため）。ほかのテストの行が混ざるため、各テストは一意の値で作ったものだけを見る
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
	operators, err = testhelper.NewOperators(ctx, app.StudioDB, key)
	if err != nil {
		return fail(err)
	}
	endpoint = app.URL + "/studio/graphql"
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

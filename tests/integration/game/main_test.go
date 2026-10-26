// game 用 GraphQL の結合テスト。本番と同じ組み立て（internal/wiring）でアプリを動かし、HTTP で呼ぶ
package game_test

import (
	"context"
	"database/sql"
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
	// 共有のアプリの game の DB と、そこに公開した見本のマスタ
	gameDB *sql.DB
	master testhelper.Master
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
	gameDB = app.GameDB
	// 共有のアプリでは、最初に公開しておく（登録した全員にスターターが配られる）。公開前の登録は、独立したアプリで確かめる
	master, err = testhelper.PublishMaster(ctx, app.GameDB)
	if err != nil {
		return fail(err)
	}
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

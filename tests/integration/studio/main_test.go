// studio 用 GraphQL の結合テスト。本番と同じ組み立て（internal/wiring）でアプリを動かし、HTTP で呼ぶ
package studio_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/wiring"
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
	if os.Getenv("DB_DSN") == "" {
		fmt.Fprintln(os.Stderr, "DB_DSN is not set. run via 'make test'")
		return 0
	}
	errorLogs := testhelper.RecordErrorLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, dsn, drop, err := rdbtest.CloneDatabase(ctx)
	if err != nil {
		return fail(err)
	}
	defer drop()
	key, err = idptest.GenerateKey("k1")
	if err != nil {
		return fail(err)
	}
	jwks, closeJWKS := idptest.StartJWKSServer(key)
	defer closeJWKS()
	verifier, err := idp.NewVerifier(ctx, jwks.URL, idptest.Issuer, idptest.Audience)
	if err != nil {
		return fail(err)
	}
	studioConfig := studio.Config{Verifier: verifier, Issuer: idptest.Issuer}
	eventDriven := eventdriven.Options{WorkerCount: 4, PollInterval: time.Second}
	app, err := wiring.Start(ctx, wiring.Config{
		DB:                   db,
		DSN:                  dsn,
		Version:              "test",
		Studio:               studioConfig,
		InitialOperatorEmail: "initial@example.com",
		EventDriven:          eventDriven,
	})
	if err != nil {
		return fail(err)
	}
	operators, err = testhelper.NewOperators(ctx, db, key)
	if err != nil {
		app.Stop()
		return fail(err)
	}
	server := httptest.NewServer(app.Handler)
	endpoint = server.URL + "/studio/graphql"
	client = testhelper.GraphQLClient{URL: endpoint}

	code := m.Run()

	server.Close()
	app.Stop()
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

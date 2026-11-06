package testhelper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SSE（graphql-sse）で受け取った 1 件。next は結果（data）、complete は Subscription の終わり
type SSEEvent struct {
	Name string
	Body map[string]any
}

// つないだままの Subscription。読み切るまで、または Close までつながる
type SSEStream struct {
	events chan SSEEvent
	cancel context.CancelFunc
}

// Subscription を SSE でつなぐ（POST で Accept: text/event-stream）。HTTP の状態が 200 でなければテストを失敗させる
// （認証の誤りは、ストリームを始める前の HTTP のエラーで返る）。テストが終わったらつなぎを閉じる
func (c GraphQLClient) Subscribe(t *testing.T, token, query string, variables map[string]any) *SSEStream {
	t.Helper()
	status, stream, err := c.SubscribeTry(t.Context(), token, query, variables)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	t.Cleanup(stream.Close)
	return stream
}

// Subscribe と同じ。HTTP の状態を返す（200 以外ではストリームは nil）
func (c GraphQLClient) SubscribeTry(ctx context.Context, token, query string, variables map[string]any) (int, *SSEStream, error) {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return 0, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(payload))
	if err != nil {
		cancel()
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return 0, nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		cancel()
		return res.StatusCode, nil, nil
	}
	stream := &SSEStream{events: make(chan SSEEvent, 64), cancel: cancel}
	go func() {
		defer close(stream.events)
		defer res.Body.Close()
		scanner := bufio.NewScanner(res.Body)
		var name, data string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if name != "" {
					event := SSEEvent{Name: name}
					if data != "" {
						_ = json.Unmarshal([]byte(data), &event.Body)
					}
					select {
					case stream.events <- event:
					case <-ctx.Done():
						return
					}
				}
				name, data = "", ""
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
	}()
	return http.StatusOK, stream, nil
}

// 次の結果（next）の data[field] を返す。errors があれば失敗させる
func (s *SSEStream) Next(t *testing.T, field string) map[string]any {
	t.Helper()
	select {
	case event, ok := <-s.events:
		require.True(t, ok)
		require.Equal(t, "next", event.Name)
		require.Nil(t, event.Body["errors"])
		data, ok := event.Body["data"].(map[string]any)[field].(map[string]any)
		require.True(t, ok)
		return data
	case <-time.After(waitTimeout):
		require.FailNow(t, "timed out waiting for the next result")
		return nil
	}
}

// 結果が来ないことを確かめる（d の間、何も届かない）
func (s *SSEStream) RequireNoResultWithin(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case <-s.events:
		require.FailNow(t, "unexpected stream activity")
	case <-time.After(d):
	}
}

// 終わりの合図（complete）が来て、サーバーが閉じるのを確かめる
func (s *SSEStream) RequireCompleted(t *testing.T) {
	t.Helper()
	timeout := time.After(waitTimeout)
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				require.FailNow(t, "stream closed without complete")
			}
			if event.Name == "complete" {
				return
			}
		case <-timeout:
			require.FailNow(t, "timed out waiting for complete")
		}
	}
}

func (s *SSEStream) Close() {
	s.cancel()
}

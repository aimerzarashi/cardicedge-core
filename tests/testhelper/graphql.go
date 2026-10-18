package testhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// GraphQL の入口（/studio/graphql 等）に POST する
type GraphQLClient struct {
	URL string
}

// HTTP の状態と、JSON をそのまま読んだ本文を返す（エラーの形も確かめるため、data に絞らない）
func (c GraphQLClient) Post(t *testing.T, token, query string, variables map[string]any) (int, map[string]any) {
	t.Helper()
	status, body, err := c.post(t.Context(), token, query, variables)
	require.NoError(t, err)
	return status, body
}

// 操作が成功した（HTTP 200 で errors が無い）ことを確かめ、data の field を返す
func (c GraphQLClient) Data(t *testing.T, token, query string, variables map[string]any, field string) map[string]any {
	t.Helper()
	data, err := c.TryData(t.Context(), token, query, variables, field)
	require.NoError(t, err)
	return data
}

// Data と同じ。Eventually の中（別の goroutine）で使うため、テストを失敗させずにエラーを返す
func (c GraphQLClient) TryData(ctx context.Context, token, query string, variables map[string]any, field string) (map[string]any, error) {
	status, body, err := c.post(ctx, token, query, variables)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK || body["errors"] != nil {
		return nil, fmt.Errorf("graphql failed: status %d, body %v", status, body)
	}
	data, ok := body["data"].(map[string]any)[field].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("graphql returned no %s: %v", field, body)
	}
	return data, nil
}

func (c GraphQLClient) post(ctx context.Context, token, query string, variables map[string]any) (int, map[string]any, error) {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return 0, nil, err
	}
	return res.StatusCode, body, nil
}

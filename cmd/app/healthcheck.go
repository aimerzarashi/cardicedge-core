package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const healthcheckTimeout = 3 * time.Second

// checkHealth は url に GET し、200 以外ならエラーを返す。
// シェルや curl の無い本番イメージで、compose の healthcheck から `app healthcheck` として使う。
func checkHealth(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status from %s: %d", url, resp.StatusCode)
	}
	return nil
}

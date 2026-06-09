package taskstore

import (
	"database/sql"
	"fmt"
	"testing"
)

func seedRepositoryTestAccount(t testing.TB, db *sql.DB, id int64) int64 {
	t.Helper()
	channelID := id + 10000
	if _, err := db.Exec(`
INSERT INTO channels (id, type, name, api_url, status)
VALUES ($1, 'openai-compatible', $2, $3, 1)
ON CONFLICT (id) DO NOTHING`,
		channelID,
		fmt.Sprintf("repository-test-channel-%d", channelID),
		fmt.Sprintf("https://example.com/repository-test-channel-%d", channelID),
	); err != nil {
		t.Fatalf("seed channel %d failed: %v", channelID, err)
	}
	if _, err := db.Exec(`
INSERT INTO api_accounts (id, name, channel_id, platform, api_type, proxy_url, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, $2, $3, 'openai-compatible', 'openai-compatible', 'https://example.com', 3, 120, '', 1, 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("repository-test-account-%d", id),
		channelID,
	); err != nil {
		t.Fatalf("seed account %d failed: %v", id, err)
	}
	return id
}

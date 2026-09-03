package postgres

import (
	"database/sql"
	"fmt"
	"testing"
)

func seedRepositoryTestAccount(t testing.TB, db *sql.DB, id int64) int64 {
	t.Helper()
	channelID := id + 10000
	if _, err := db.Exec(`
INSERT INTO provider_endpoints (id, api_type, name, base_url, status)
VALUES ($1, 'openai-compatible', $2, $3, 1)
ON CONFLICT (id) DO NOTHING`,
		channelID,
		fmt.Sprintf("repository-test-channel-%d", channelID),
		fmt.Sprintf("https://example.com/repository-test-channel-%d", channelID),
	); err != nil {
		t.Fatalf("seed channel %d failed: %v", channelID, err)
	}
	if _, err := db.Exec(`
INSERT INTO api_accounts (id, name, provider_endpoint_id, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, $2, $3, 3, 120, '', 1, 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("repository-test-account-%d", id),
		channelID,
	); err != nil {
		t.Fatalf("seed account %d failed: %v", id, err)
	}
	if _, err := db.Exec(`
INSERT INTO api_account_secrets (account_id, secret_material_kind, secret_payload, key_version, fingerprint, status)
VALUES ($1, 'encrypted', $2, 'test-key-v1', $3, 'active')
ON CONFLICT (account_id) WHERE status = 'active' DO NOTHING`,
		id,
		fmt.Sprintf("encrypted-secret-%d", id),
		fmt.Sprintf("repository-test-fingerprint-%d", id),
	); err != nil {
		t.Fatalf("seed account credential %d failed: %v", id, err)
	}
	return id
}

func loadRepositoryTestCredentialID(t testing.TB, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var credentialID int64
	if err := db.QueryRow(`
SELECT id
FROM api_account_secrets
WHERE account_id = $1 AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, accountID).Scan(&credentialID); err != nil {
		t.Fatalf("load account %d credential failed: %v", accountID, err)
	}
	return credentialID
}

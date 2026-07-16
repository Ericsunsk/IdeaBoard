package rewards

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	creditsloadtest "ideaboard/api/internal/credits/loadtestsupport"
	"ideaboard/api/internal/testdb"
)

const (
	minimumRewardLoadRedemptions  = 1_000_000
	rewardLoadHistoryCount        = 200
	rewardLoadBatchCount          = 1_000
	rewardLoadBatchCodeCount      = 10
	rewardLoadUserBase            = int64(2_000_000)
	rewardLoadCampaignID          = int64(9_000_001)
	rewardLoadCodeID              = int64(9_000_001)
	rewardLoadHistoryCampaignBase = int64(9_100_000)
	rewardLoadBatchCampaignID     = int64(9_200_000)
	rewardLoadBatchBase           = int64(9_300_000)
	rewardLoadBatchCodeBase       = int64(9_400_000)
	rewardLoadSystemAccountID     = int64(19_000_000)
	rewardLoadAvailableBase       = int64(20_000_000)
	rewardLoadReservedBase        = int64(22_000_000)
)

// TestRewardMillionRedemptionDataset is intentionally opt-in because it creates
// a fresh isolated schema with at least one million complete Redemption facts.
func TestRewardMillionRedemptionDataset(t *testing.T) {
	if os.Getenv("REWARDS_LOAD_TEST") != "1" {
		t.Skip("set REWARDS_LOAD_TEST=1 to run the million-Redemption Rewards harness")
	}
	redemptionCount := rewardLoadRedemptionCount(t)
	db, pool := testdb.OpenWithPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	keyring := newTestRewardKeyring(t)
	service := NewService(pool, credits.NewService(pool), keyring)

	seedStarted := time.Now()
	seedRewardMillionDataset(t, ctx, db, keyring, redemptionCount)
	seedDuration := time.Since(seedStarted)
	verifyRewardMillionFacts(t, ctx, db, redemptionCount)
	analyzeRewardLoadTables(t, ctx, db)

	targetUserID := rewardLoadUserBase + 1
	lookupLatency := sampleRewardLoadLatency(t, 20, func() error {
		_, err := service.LookupCode(ctx, "MILLIONIDEA2026")
		return err
	})
	userLatency := sampleRewardLoadLatency(t, 20, func() error {
		_, err := service.ListRedemptions(ctx, ListRedemptionsInput{UserID: targetUserID, Limit: 50})
		return err
	})
	adminLatency := sampleRewardLoadLatency(t, 20, func() error {
		_, err := service.ListAdminRedemptions(ctx, ListAdminRedemptionsInput{CampaignID: rewardLoadCampaignID, Limit: 50})
		return err
	})

	assertRewardLoadPlans(t, ctx, db, keyring, targetUserID, redemptionCount)
	var postgresVersion string
	if err := db.QueryRowContext(ctx, `SHOW server_version`).Scan(&postgresVersion); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	t.Logf("rewards million result: redemptions=%d postgres=%s go=%s os=%s arch=%s cpus=%d seed=%s lookup_p50=%s lookup_p95=%s user_history_p50=%s user_history_p95=%s admin_page_p50=%s admin_page_p95=%s",
		redemptionCount, postgresVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), seedDuration,
		lookupLatency.p50, lookupLatency.p95, userLatency.p50, userLatency.p95, adminLatency.p50, adminLatency.p95)
}

func TestRewardHotPublicCampaign(t *testing.T) {
	if os.Getenv("REWARDS_LOAD_TEST") != "1" {
		t.Skip("set REWARDS_LOAD_TEST=1 to run the hot Public Campaign Rewards harness")
	}
	const (
		requestCount = 1_000
		capacity     = 800
		workerCount  = 32
		creditAmount = int64(3)
	)
	db, pool := testdb.OpenWithPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	seedHotRewardUsers(t, ctx, db, requestCount)
	creditService := credits.NewService(pool)
	service := NewService(pool, creditService, newTestRewardKeyring(t))
	campaign, err := service.CreateCampaign(ctx, CreateCampaignCommand{
		DisplayName: "Hot Public Campaign", CreditAmount: creditAmount, MaxRedemptions: capacity,
		ActorUserID: 1, Reason: "Rewards hot benchmark",
	})
	if err != nil {
		t.Fatalf("create hot Campaign: %v", err)
	}
	issued, err := service.IssuePublicCode(ctx, IssuePublicCodeCommand{
		CampaignID: campaign.ID, Code: "HOTIDEA2026", ActorUserID: 1, Reason: "Rewards hot benchmark",
	})
	if err != nil {
		t.Fatalf("issue hot public Code: %v", err)
	}
	if _, err := service.ActivateCampaign(ctx, ActivateCampaignCommand{
		CampaignID: campaign.ID, ActorUserID: 1, Reason: "Rewards hot benchmark",
	}); err != nil {
		t.Fatalf("activate hot Campaign: %v", err)
	}

	type hotRequest struct{ userID int64 }
	type hotResult struct {
		duration time.Duration
		err      error
	}
	requests := make(chan hotRequest, workerCount)
	results := make(chan hotResult, requestCount)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for request := range requests {
				started := time.Now()
				_, err := service.Redeem(ctx, RedeemCommand{UserID: request.userID, Code: issued.Plaintext})
				results <- hotResult{duration: time.Since(started), err: err}
			}
		}()
	}
	lockStats := startRewardLockSampler(ctx, db)
	started := time.Now()
	ticker := time.NewTicker(10 * time.Millisecond)
	for index := 0; index < requestCount; index++ {
		<-ticker.C
		requests <- hotRequest{userID: 10_000 + int64(index)}
	}
	ticker.Stop()
	close(requests)
	workers.Wait()
	close(results)
	duration := time.Since(started)
	lockSamples, peakLockWaiters := lockStats.stop()

	latencies := make([]time.Duration, 0, requestCount)
	successes := 0
	denied := 0
	for result := range results {
		latencies = append(latencies, result.duration)
		switch {
		case result.err == nil:
			successes++
		case errors.Is(result.err, ErrCampaignExhausted), errors.Is(result.err, ErrCodeUnavailable):
			denied++
		default:
			t.Fatalf("hot Redeem() unexpected error: %v", result.err)
		}
	}
	if successes != capacity || denied != requestCount-capacity {
		t.Fatalf("hot results successes=%d denied=%d, want %d/%d", successes, denied, capacity, requestCount-capacity)
	}
	assertHotRewardFacts(t, ctx, db, campaign.ID, capacity, creditAmount)
	throughput := float64(requestCount) / duration.Seconds()
	if throughput < 90 {
		t.Fatalf("hot Campaign throughput %.1f requests/s, want >= 90", throughput)
	}
	latency := summarizeRewardLoadLatency(latencies)
	t.Logf("rewards hot result: requests=%d capacity=%d duration=%s throughput_requests_per_second=%.1f p50=%s p95=%s p99=%s lock_wait_samples=%d peak_lock_waiters=%d",
		requestCount, capacity, duration, throughput, latency.p50, latency.p95, latency.p99, lockSamples, peakLockWaiters)
}

func rewardLoadRedemptionCount(t *testing.T) int {
	t.Helper()
	raw := os.Getenv("REWARDS_LOAD_TEST_REDEMPTIONS")
	if raw == "" {
		return minimumRewardLoadRedemptions
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < minimumRewardLoadRedemptions {
		t.Fatalf("REWARDS_LOAD_TEST_REDEMPTIONS must be an integer >= %d", minimumRewardLoadRedemptions)
	}
	return count
}

func seedRewardMillionDataset(t *testing.T, ctx context.Context, db *sql.DB, keyring CodeKeyring, count int) {
	t.Helper()
	digest := keyring.issueDigest("MILLIONIDEA2026")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin Rewards load seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	userStatements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, username, storage_uid, role, status)
SELECT $1 + series, 'reward-load-' || series, upper(lpad(to_hex(series - 1), 6, '0')), 'user', 1
FROM generate_series(1, $2::BIGINT) series`, []any{rewardLoadUserBase, count}},
		{`INSERT INTO users_used_uids (storage_uid)
SELECT upper(lpad(to_hex(series - 1), 6, '0')) FROM generate_series(1, $1::BIGINT) series`, []any{count}},
	}
	for _, statement := range userStatements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed Rewards million users: %v", err)
		}
	}
	if err := creditsloadtest.SeedSequentialGrantFacts(ctx, tx, creditsloadtest.SequentialGrantFactsConfig{
		BaseUserID:           rewardLoadUserBase,
		Count:                count,
		SystemAccountID:      rewardLoadSystemAccountID,
		AvailableAccountBase: rewardLoadAvailableBase,
		ReservedAccountBase:  rewardLoadReservedBase,
	}); err != nil {
		t.Fatalf("seed Rewards million Credit facts: %v", err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO reward_campaigns (id, display_name, internal_note, credit_amount, mode, status, max_redemptions, redemption_count, created_by)
VALUES ($1, 'Million Redemption Campaign', 'Rewards load harness', 1, 'public', 'active', $2, 0, 1)`, []any{rewardLoadCampaignID, count}},
		{`INSERT INTO reward_codes (id, campaign_id, key_version, code_digest, code_hint, issued_by)
VALUES ($1, $2, $3, $4, '2026', 1)`, []any{rewardLoadCodeID, rewardLoadCampaignID, digest.version, digest.digest}},
		{`ALTER TABLE reward_redemptions DISABLE TRIGGER reward_redemptions_single_use_once`, nil},
		{`ALTER TABLE reward_redemptions DISABLE TRIGGER reward_redemptions_consume_single_use_code`, nil},
		{`INSERT INTO reward_redemptions (
  id, campaign_id, code_id, user_id, credit_amount, credit_transaction_id, available_balance_after, redeemed_at
)
SELECT series, $1, $2, $3 + series, 1, 'reward_load_tx_' || series, 1,
       now() - (($4 - series)::TEXT || ' microseconds')::INTERVAL
FROM generate_series(1, $4::BIGINT) series`, []any{rewardLoadCampaignID, rewardLoadCodeID, rewardLoadUserBase, count}},
		{`ALTER TABLE reward_redemptions ENABLE TRIGGER reward_redemptions_single_use_once`, nil},
		{`ALTER TABLE reward_redemptions ENABLE TRIGGER reward_redemptions_consume_single_use_code`, nil},
		{`UPDATE reward_campaigns SET redemption_count = $2 WHERE id = $1`, []any{rewardLoadCampaignID, count}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed Rewards million dataset: %v", err)
		}
	}
	seedRewardHistoryAndBatches(t, ctx, tx, count)
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit Rewards load seed: %v", err)
	}
}

func seedRewardHistoryAndBatches(t *testing.T, ctx context.Context, tx *sql.Tx, count int) {
	t.Helper()
	targetUserID := rewardLoadUserBase + 1
	if err := creditsloadtest.SeedSingleWalletGrantFacts(ctx, tx, creditsloadtest.SingleWalletGrantFactsConfig{
		UserID:                targetUserID,
		Count:                 rewardLoadHistoryCount,
		SystemAccountID:       rewardLoadSystemAccountID,
		AvailableAccountID:    rewardLoadAvailableBase + 1,
		BusinessReferenceBase: int64(count),
	}); err != nil {
		t.Fatalf("seed Rewards history Credit facts: %v", err)
	}
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO reward_campaigns (id, display_name, credit_amount, mode, status, max_redemptions, redemption_count, created_by)
SELECT $1 + series, 'History Campaign ' || series, 1, 'public', 'active', 1, 0, 1
FROM generate_series(1, $2::BIGINT) series`, []any{rewardLoadHistoryCampaignBase, rewardLoadHistoryCount}},
		{`INSERT INTO reward_codes (id, campaign_id, key_version, code_digest, code_hint, issued_by)
SELECT $1 + series, $2 + series, 'load-v1', decode(lpad(to_hex($1 + series), 64, '0'), 'hex'),
       upper(lpad(to_hex(series), 4, '0')), 1
FROM generate_series(1, $3::BIGINT) series`, []any{rewardLoadHistoryCampaignBase, rewardLoadHistoryCampaignBase, rewardLoadHistoryCount}},
		{`ALTER TABLE reward_redemptions DISABLE TRIGGER reward_redemptions_single_use_once`, nil},
		{`ALTER TABLE reward_redemptions DISABLE TRIGGER reward_redemptions_consume_single_use_code`, nil},
		{`INSERT INTO reward_redemptions (
  id, campaign_id, code_id, user_id, credit_amount, credit_transaction_id, available_balance_after
)
SELECT $1 + series, $2 + series, $2 + series, $3, 1,
       'reward_history_tx_' || series, series + 1
FROM generate_series(1, $4::BIGINT) series`, []any{count, rewardLoadHistoryCampaignBase, targetUserID, rewardLoadHistoryCount}},
		{`ALTER TABLE reward_redemptions ENABLE TRIGGER reward_redemptions_single_use_once`, nil},
		{`ALTER TABLE reward_redemptions ENABLE TRIGGER reward_redemptions_consume_single_use_code`, nil},
		{`UPDATE reward_campaigns SET redemption_count = 1 WHERE id > $1 AND id <= $1 + $2`, []any{rewardLoadHistoryCampaignBase, rewardLoadHistoryCount}},
		{`INSERT INTO reward_campaigns (id, display_name, credit_amount, mode, status, max_redemptions, redemption_count, created_by)
VALUES ($1, 'Batch Read Campaign', 1, 'single_use', 'active', 0, 0, 1)`, []any{rewardLoadBatchCampaignID}},
		{`INSERT INTO reward_code_batches (id, campaign_id, idempotency_key, request_fingerprint, code_count, issued_by)
SELECT $1 + series, $2, 'reward-load-batch-' || series,
       decode(lpad(to_hex($1 + series), 64, '0'), 'hex'), $3, 1
FROM generate_series(1, $4::BIGINT) series`, []any{rewardLoadBatchBase, rewardLoadBatchCampaignID, rewardLoadBatchCodeCount, rewardLoadBatchCount}},
		{`INSERT INTO reward_codes (id, campaign_id, batch_id, key_version, code_digest, code_hint, issued_by)
SELECT $1 + series, $2, $3 + ((series - 1) / $4), 'load-v1',
       decode(lpad(to_hex($1 + series), 64, '0'), 'hex'), upper(lpad(to_hex((series - 1) % 65536), 4, '0')), 1
FROM generate_series(1, $4::BIGINT * $5::BIGINT) series`, []any{rewardLoadBatchCodeBase, rewardLoadBatchCampaignID, rewardLoadBatchBase + 1, rewardLoadBatchCodeCount, rewardLoadBatchCount}},
	}
	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query.query, query.args...); err != nil {
			t.Fatalf("seed Rewards history/Batch facts: %v", err)
		}
	}
}

func verifyRewardMillionFacts(t *testing.T, ctx context.Context, db *sql.DB, count int) {
	t.Helper()
	var redemptions, distinctUsers, transactions, invalidTransactions, invalidGrantFacts, history int64
	err := db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM reward_redemptions WHERE campaign_id = $1),
  (SELECT COUNT(DISTINCT user_id) FROM reward_redemptions WHERE campaign_id = $1),
  (SELECT COUNT(*) FROM credit_transactions WHERE transaction_id LIKE 'reward_load_tx_%'),
  (SELECT COUNT(*) FROM (
     SELECT transactions.transaction_id
     FROM credit_transactions transactions
     JOIN credit_postings postings ON postings.transaction_id = transactions.transaction_id
     WHERE transactions.transaction_id LIKE 'reward_%_tx_%'
     GROUP BY transactions.transaction_id, transactions.posting_count
     HAVING COUNT(*) <> transactions.posting_count OR SUM(postings.amount) <> 0
   ) invalid),
  (SELECT COUNT(*)
   FROM reward_redemptions redemptions
   JOIN credit_transactions transactions
     ON transactions.transaction_id = redemptions.credit_transaction_id
   WHERE transactions.source <> 'reward_redemption'
      OR transactions.reason <> '兑换奖励'
      OR transactions.business_reference_type <> 'reward_redemption'
      OR transactions.business_reference_id <> redemptions.id::TEXT),
  (SELECT COUNT(*) FROM reward_redemptions WHERE user_id = $2)`, rewardLoadCampaignID, rewardLoadUserBase+1).Scan(
		&redemptions, &distinctUsers, &transactions, &invalidTransactions, &invalidGrantFacts, &history)
	if err != nil {
		t.Fatalf("verify Rewards million facts: %v", err)
	}
	if redemptions != int64(count) || distinctUsers != int64(count) || transactions != int64(count) ||
		invalidTransactions != 0 || invalidGrantFacts != 0 || history != rewardLoadHistoryCount+1 {
		t.Fatalf("invalid Rewards facts redemptions=%d users=%d transactions=%d invalid_transactions=%d invalid_grants=%d history=%d",
			redemptions, distinctUsers, transactions, invalidTransactions, invalidGrantFacts, history)
	}
}

func analyzeRewardLoadTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, table := range []string{"reward_campaigns", "reward_code_batches", "reward_codes", "reward_redemptions"} {
		if _, err := db.ExecContext(ctx, `ANALYZE `+table); err != nil {
			t.Fatalf("ANALYZE %s: %v", table, err)
		}
	}
}

type rewardExplainExpectation struct {
	name     string
	query    string
	args     []any
	required []string
	anyOf    []string
}

func assertRewardLoadPlans(t *testing.T, ctx context.Context, db *sql.DB, keyring CodeKeyring, userID int64, count int) {
	t.Helper()
	digest := keyring.issueDigest("MILLIONIDEA2026")
	now := time.Now().UTC()
	checks := []rewardExplainExpectation{
		{name: "exact dual-HMAC Code lookup", query: adminCodeLookupSQL,
			args:     []any{digest.version, digest.digest, "load-previous", make([]byte, 32), maskedCodeHintPrefix},
			required: []string{"uq_reward_codes_digest", "idx_reward_redemptions_code_id_desc"}},
		{name: "User history", query: redemptionHistorySQL,
			args: []any{userID, count / 2, 51}, required: []string{"idx_reward_redemptions_user_id"}},
		{name: "Admin Redemption pagination", query: adminRedemptionListSQL,
			args:  []any{rewardLoadCampaignID, count / 2, 51, maskedCodeHintPrefix},
			anyOf: []string{"idx_reward_redemptions_campaign_id", "reward_redemptions_pkey"}},
		{name: "Campaign mode cursor list", query: campaignListSQL,
			args:     []any{rewardLoadBatchCampaignID + 1, 21, "", CampaignModeSingleUse, "", now},
			required: []string{"idx_reward_campaigns_mode_id"}},
		{name: "Campaign search and derived status list", query: campaignListSQL,
			args: []any{rewardLoadBatchCampaignID + 1, 21, "History", CampaignModePublic, CampaignStatusActive, now}},
		{name: "Campaign detail", query: `SELECT ` + campaignProjectionSQL + ` FROM reward_campaigns WHERE id = $1`,
			args: []any{rewardLoadCampaignID}, required: []string{"reward_campaigns_pkey"}},
		{name: "Batch reads", query: codeBatchMetadataSQL + `
WHERE batches.campaign_id = $1 AND ($2::BIGINT <= 0 OR batches.id < $2)
ORDER BY batches.id DESC
LIMIT $3`, args: []any{rewardLoadBatchCampaignID, rewardLoadBatchBase + rewardLoadBatchCount + 1, 21},
			required: []string{"idx_reward_codes_batch_id", "idx_reward_redemptions_code_id_desc"},
			anyOf:    []string{"idx_reward_code_batches_campaign_id", "reward_code_batches_pkey"}},
	}
	for _, check := range checks {
		plan := explainRewardLoadQuery(t, ctx, db, check.query, check.args...)
		for _, indexName := range check.required {
			if !rewardPlanUsesIndex(plan, indexName) {
				encoded, _ := json.MarshalIndent(plan, "", "  ")
				t.Fatalf("%s plan did not use %s:\n%s", check.name, indexName, encoded)
			}
		}
		if len(check.anyOf) > 0 && !rewardPlanUsesAnyIndex(plan, check.anyOf) {
			encoded, _ := json.MarshalIndent(plan, "", "  ")
			t.Fatalf("%s plan did not use one of %v:\n%s", check.name, check.anyOf, encoded)
		}
		if rewardPlanHasLargeSeqScan(plan) {
			encoded, _ := json.MarshalIndent(plan, "", "  ")
			t.Fatalf("%s plan contains an unbounded large-table Seq Scan:\n%s", check.name, encoded)
		}
	}
}

func rewardPlanUsesAnyIndex(plan any, indexNames []string) bool {
	for _, indexName := range indexNames {
		if rewardPlanUsesIndex(plan, indexName) {
			return true
		}
	}
	return false
}

func explainRewardLoadQuery(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) any {
	t.Helper()
	var raw []byte
	if err := db.QueryRowContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, args...).Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN Rewards load query: %v", err)
	}
	var plan any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode Rewards EXPLAIN: %v", err)
	}
	return plan
}

func rewardPlanUsesIndex(value any, indexName string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typed["Index Name"] == indexName {
			return true
		}
		for _, child := range typed {
			if rewardPlanUsesIndex(child, indexName) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if rewardPlanUsesIndex(child, indexName) {
				return true
			}
		}
	}
	return false
}

func rewardPlanHasLargeSeqScan(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typed["Node Type"] == "Seq Scan" {
			relation, _ := typed["Relation Name"].(string)
			if relation == "reward_redemptions" || relation == "reward_codes" {
				return true
			}
		}
		for _, child := range typed {
			if rewardPlanHasLargeSeqScan(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if rewardPlanHasLargeSeqScan(child) {
				return true
			}
		}
	}
	return false
}

func seedHotRewardUsers(t *testing.T, ctx context.Context, db *sql.DB, count int) {
	t.Helper()
	statements := []string{
		`INSERT INTO users (id, username, storage_uid, role, status)
SELECT 9999 + series, 'reward-hot-' || series, upper('H' || lpad(to_hex(series), 5, '0')), 'user', 1
FROM generate_series(1, $1::BIGINT) series`,
		`INSERT INTO users_used_uids (storage_uid)
SELECT upper('H' || lpad(to_hex(series), 5, '0')) FROM generate_series(1, $1::BIGINT) series`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, count); err != nil {
			t.Fatalf("seed hot Reward users: %v", err)
		}
	}
	if err := creditsloadtest.SeedSequentialWallets(ctx, db, 10_000, count); err != nil {
		t.Fatalf("seed hot Reward Wallets: %v", err)
	}
}

func assertHotRewardFacts(t *testing.T, ctx context.Context, db *sql.DB, campaignID int64, capacity int, amount int64) {
	t.Helper()
	var projected, redemptions, distinctUsers, grants, balance int64
	err := db.QueryRowContext(ctx, `
SELECT
  (SELECT redemption_count FROM reward_campaigns WHERE id = $1),
  (SELECT COUNT(*) FROM reward_redemptions WHERE campaign_id = $1),
  (SELECT COUNT(DISTINCT user_id) FROM reward_redemptions WHERE campaign_id = $1),
  (SELECT COUNT(*) FROM credit_transactions WHERE source = 'reward_redemption'),
  (SELECT COALESCE(SUM(balance), 0) FROM credit_accounts WHERE wallet_user_id BETWEEN 10000 AND 10999 AND account_role = 'user_available')`, campaignID).Scan(
		&projected, &redemptions, &distinctUsers, &grants, &balance)
	if err != nil {
		t.Fatalf("verify hot Reward facts: %v", err)
	}
	want := int64(capacity)
	if projected != want || redemptions != want || distinctUsers != want || grants != want || balance != want*amount {
		t.Fatalf("hot Reward facts projected=%d redemptions=%d users=%d grants=%d balance=%d, want %d/%d/%d/%d/%d",
			projected, redemptions, distinctUsers, grants, balance, want, want, want, want, want*amount)
	}
}

type rewardLockSampler struct {
	stopCh  chan struct{}
	doneCh  chan struct{}
	samples atomic.Int64
	peak    atomic.Int64
}

func startRewardLockSampler(ctx context.Context, db *sql.DB) *rewardLockSampler {
	sampler := &rewardLockSampler{stopCh: make(chan struct{}), doneCh: make(chan struct{})}
	go func() {
		defer close(sampler.doneCh)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-sampler.stopCh:
				return
			case <-ticker.C:
				var waiters int64
				err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid()
  AND wait_event_type = 'Lock' AND query LIKE '%reward_campaigns%'`).Scan(&waiters)
				if err != nil || waiters == 0 {
					continue
				}
				sampler.samples.Add(1)
				for current := sampler.peak.Load(); waiters > current && !sampler.peak.CompareAndSwap(current, waiters); current = sampler.peak.Load() {
				}
			}
		}
	}()
	return sampler
}

func (s *rewardLockSampler) stop() (int64, int64) {
	close(s.stopCh)
	<-s.doneCh
	return s.samples.Load(), s.peak.Load()
}

type rewardLoadLatency struct {
	p50 time.Duration
	p95 time.Duration
	p99 time.Duration
}

func sampleRewardLoadLatency(t *testing.T, samples int, operation func() error) rewardLoadLatency {
	t.Helper()
	durations := make([]time.Duration, 0, samples)
	for sample := 0; sample < samples; sample++ {
		started := time.Now()
		if err := operation(); err != nil {
			t.Fatalf("sample Rewards load query: %v", err)
		}
		durations = append(durations, time.Since(started))
	}
	return summarizeRewardLoadLatency(durations)
}

func summarizeRewardLoadLatency(durations []time.Duration) rewardLoadLatency {
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	percentile := func(value float64) time.Duration {
		index := int(float64(len(sorted)-1) * value)
		return sorted[index]
	}
	return rewardLoadLatency{p50: percentile(0.50), p95: percentile(0.95), p99: percentile(0.99)}
}

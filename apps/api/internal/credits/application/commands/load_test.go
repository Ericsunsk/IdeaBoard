package commands_test

import (
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/testhelpers"

	"context"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/testenv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const minimumCreditLoadTransactions = 1_000_000

// TestCreditMillionTransactionDataset is an explicit load harness, not part
// of the default suite. Run it with CREDITS_LOAD_TEST=1 and
// API_TEST_DATABASE_DSN set. It bulk-seeds immutable, balanced facts, verifies
// them independently, then exercises the public read and contention seams.
func TestCreditMillionTransactionDataset(t *testing.T) {
	if testenv.Get("CREDITS_LOAD_TEST") != "1" {
		t.Skip("set CREDITS_LOAD_TEST=1 to run the million-transaction Credits harness")
	}
	transactionCount := creditLoadTransactionCount(t)
	db, pool := testdb.OpenWithPool(t)
	service := credits.NewService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	userID := int64(95)
	if _, err := service.Grant(ctx, ports.GrantCommand{
		UserID:   userID,
		Amount:   testhelpers.MustCreditAmount(t, 1),
		Metadata: testhelpers.TestMetadata("load:bootstrap", "load-bootstrap"),
	}); err != nil {
		t.Fatalf("bootstrap Grant() error = %v", err)
	}

	var availableAccountID int64
	var issuanceAccountID int64
	if err := db.QueryRow(ctx, `
SELECT id FROM credit_accounts
WHERE wallet_user_id = $1 AND account_role = 'user_available'`, userID).Scan(&availableAccountID); err != nil {
		t.Fatalf("load available account error = %v", err)
	}
	if err := db.QueryRow(ctx, `
SELECT id FROM credit_accounts
WHERE wallet_user_id IS NULL AND shard_key = $1 AND account_role = 'system_issuance'`, testhelpers.WalletSystemShard(userID)).Scan(&issuanceAccountID); err != nil {
		t.Fatalf("load issuance account error = %v", err)
	}

	seedStarted := time.Now()
	if _, err := db.Exec(ctx, `
ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert;
ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope;`); err != nil {
		t.Fatalf("disable seed triggers error = %v", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count,
  available_balance_after, reserved_balance_after, created_at
)
SELECT
  'ctx_load_' || series::TEXT,
  $1,
  'grant',
  'load:grant:' || series::TEXT,
  lpad(to_hex(series), 64, '0'),
  'system',
  'load-harness',
  'load_test',
  'million transaction dataset',
  'load_test',
  series::TEXT,
  1,
  2,
  series + 1,
  0,
  now() - (($2 - series)::TEXT || ' microseconds')::INTERVAL
FROM generate_series(1, $2::BIGINT) AS series`, userID, transactionCount); err != nil {
		t.Fatalf("seed credit transactions error = %v", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
SELECT
  'ctx_load_' || series::TEXT,
  posting.account_id,
  posting.amount
FROM generate_series(1, $1::BIGINT) AS series
CROSS JOIN LATERAL (
  VALUES ($2::BIGINT, -1::BIGINT), ($3::BIGINT, 1::BIGINT)
) AS posting(account_id, amount)`, transactionCount, issuanceAccountID, availableAccountID); err != nil {
		t.Fatalf("seed credit postings error = %v", err)
	}
	if _, err := db.Exec(ctx, `
ALTER TABLE credit_transactions ENABLE TRIGGER validate_credit_transaction_on_insert;
ALTER TABLE credit_postings ENABLE TRIGGER guard_credit_posting_transaction_scope;`); err != nil {
		t.Fatalf("enable seed triggers error = %v", err)
	}
	seedDuration := time.Since(seedStarted)

	verifyStarted := time.Now()
	var invalidTransactions int64
	if err := db.QueryRow(ctx, `
SELECT COUNT(*)
FROM (
  SELECT transaction_id
  FROM credit_postings
  WHERE transaction_id LIKE 'ctx_load_%'
  GROUP BY transaction_id
  HAVING COUNT(*) <> 2 OR SUM(amount) <> 0 OR COUNT(DISTINCT asset_code) <> 1
) invalid`).Scan(&invalidTransactions); err != nil {
		t.Fatalf("verify load ledger error = %v", err)
	}
	if invalidTransactions != 0 {
		t.Fatalf("invalid load transactions = %d", invalidTransactions)
	}
	verifyDuration := time.Since(verifyStarted)

	var databaseBytesBefore int64
	if err := db.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&databaseBytesBefore); err != nil {
		t.Fatalf("read database size before reconciliation error = %v", err)
	}
	var memoryBefore runtime.MemStats
	runtime.ReadMemStats(&memoryBefore)
	var reconciliationRunID int64
	if err := db.QueryRow(ctx, `SELECT run_id FROM start_credit_reconciliation('manual', 1, NULL, FALSE)`).Scan(&reconciliationRunID); err != nil {
		t.Fatalf("start reconciliation error = %v", err)
	}
	reconciliationStarted := time.Now()
	if err := service.ExecuteReconciliation(ctx, reconciliationRunID); err != nil {
		t.Fatalf("execute million reconciliation error = %v", err)
	}
	reconciliationDuration := time.Since(reconciliationStarted)
	var reconciliationFindings int64
	if err := db.QueryRow(ctx, `SELECT finding_count FROM credit_reconciliation_runs WHERE id = $1`, reconciliationRunID).Scan(&reconciliationFindings); err != nil {
		t.Fatalf("read reconciliation summary error = %v", err)
	}
	if reconciliationFindings != 0 {
		t.Fatalf("clean million dataset reconciliation findings = %d", reconciliationFindings)
	}
	var databaseBytesAfter int64
	if err := db.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&databaseBytesAfter); err != nil {
		t.Fatalf("read database size after reconciliation error = %v", err)
	}
	var memoryAfter runtime.MemStats
	runtime.ReadMemStats(&memoryAfter)

	walletStarted := time.Now()
	wallet, err := service.GetWallet(ctx, userID)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	walletDuration := time.Since(walletStarted)
	if wallet.Available != int64(transactionCount)+1 {
		t.Fatalf("wallet available = %d, want %d", wallet.Available, transactionCount+1)
	}
	ledgerStarted := time.Now()
	page, err := service.ListLedger(ctx, domain.ListLedgerInput{UserID: userID, Limit: 50})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	ledgerDuration := time.Since(ledgerStarted)
	if len(page.Transactions) != 50 {
		t.Fatalf("ledger page length = %d, want 50", len(page.Transactions))
	}

	contentionStarted := time.Now()
	runCreditLoadContention(t, ctx, service, userID)
	contentionDuration := time.Since(contentionStarted)
	parallelStarted := time.Now()
	runCreditLoadParallelWallets(t, ctx, service, pool)
	parallelDuration := time.Since(parallelStarted)
	throughputDuration, throughputRate := runCreditMultiWalletThroughput(t, ctx, service, db)
	if throughputRate < 232 {
		t.Fatalf("different-wallet throughput %.1f commands/s, want >= 232", throughputRate)
	}

	var postgresVersion string
	if err := db.QueryRow(ctx, `SHOW server_version`).Scan(&postgresVersion); err != nil {
		t.Fatalf("read PostgreSQL version error = %v", err)
	}
	t.Logf(
		"credits load result: transactions=%d postings=%d postgres=%s go=%s os=%s arch=%s cpus=%d seed=%s verify=%s reconciliation=%s reconciliation_findings=%d database_bytes_before=%d database_bytes_after=%d go_heap_before=%d go_heap_after=%d wallet=%s ledger50=%s same_wallet_32=%s different_wallets=%s multi_wallet_1000=%s throughput_commands_per_second=%.1f",
		transactionCount,
		transactionCount*2,
		postgresVersion,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
		seedDuration,
		verifyDuration,
		reconciliationDuration,
		reconciliationFindings,
		databaseBytesBefore,
		databaseBytesAfter,
		memoryBefore.HeapAlloc,
		memoryAfter.HeapAlloc,
		walletDuration,
		ledgerDuration,
		contentionDuration,
		parallelDuration,
		throughputDuration,
		throughputRate,
	)
}

func creditLoadTransactionCount(t *testing.T) int {
	t.Helper()
	raw := testenv.Get("CREDITS_LOAD_TEST_TRANSACTIONS")
	if raw == "" {
		return minimumCreditLoadTransactions
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < minimumCreditLoadTransactions {
		t.Fatalf("CREDITS_LOAD_TEST_TRANSACTIONS must be an integer >= %d", minimumCreditLoadTransactions)
	}
	return count
}

func runCreditLoadContention(t *testing.T, ctx context.Context, service *credits.Service, userID int64) {
	t.Helper()
	const workers = 32
	var wait sync.WaitGroup
	wait.Add(workers)
	errorsCh := make(chan error, workers)
	amount := testhelpers.MustCreditAmount(t, 1)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			defer wait.Done()
			_, err := service.Hold(ctx, ports.HoldCommand{
				UserID: userID,
				Amount: amount,
				Metadata: testhelpers.TestMetadata(
					fmt.Sprintf("load:contention:%d", worker),
					fmt.Sprintf("load-contention-%d", worker),
				),
			})
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("same-wallet contention Hold() error = %v", err)
		}
	}
}

func runCreditLoadParallelWallets(t *testing.T, ctx context.Context, service *credits.Service, pool creditPoolBeginner) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("parallel wallet Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := service.InTx(tx).Grant(ctx, ports.GrantCommand{
		UserID:   99,
		Amount:   testhelpers.MustCreditAmount(t, 1),
		Metadata: testhelpers.TestMetadata("load:parallel:first", "load-parallel-first"),
	}); err != nil {
		t.Fatalf("parallel first Grant() error = %v", err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := service.Grant(ctx, ports.GrantCommand{
			UserID:   101,
			Amount:   testhelpers.MustCreditAmount(t, 1),
			Metadata: testhelpers.TestMetadata("load:parallel:second", "load-parallel-second"),
		})
		finished <- err
	}()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("parallel second Grant() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("different-wallet Grant blocked on another wallet's open transaction")
	}
}

func runCreditMultiWalletThroughput(t *testing.T, ctx context.Context, service *credits.Service, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) (time.Duration, float64) {
	t.Helper()
	const commandCount = 1000
	if _, err := db.Exec(ctx, `
INSERT INTO users (id, display_name, storage_uid, role, status)
SELECT 10000 + series, 'credits-load-' || series, 'L' || lpad(series::TEXT, 5, '0'), 'user', 1
FROM generate_series(1, $1::BIGINT) series ON CONFLICT (id) DO NOTHING`, commandCount); err != nil {
		t.Fatalf("seed throughput users error = %v", err)
	}
	for userID := int64(10001); userID <= 10000+commandCount; userID++ {
		if _, err := service.Grant(ctx, ports.GrantCommand{UserID: userID, Amount: testhelpers.MustCreditAmount(t, 2), Metadata: testhelpers.TestMetadata(
			fmt.Sprintf("load:throughput:grant:%d", userID), fmt.Sprintf("load-throughput-grant-%d", userID),
		)}); err != nil {
			t.Fatalf("seed throughput wallet %d error = %v", userID, err)
		}
	}
	started := time.Now()
	var next atomic.Int64
	var wait sync.WaitGroup
	errorsCh := make(chan error, 64)
	for worker := 0; worker < 64; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				index := next.Add(1) - 1
				if index >= commandCount {
					return
				}
				userID := int64(10001) + index
				_, err := service.Hold(ctx, ports.HoldCommand{UserID: userID, Amount: testhelpers.MustCreditAmount(t, 1), Metadata: testhelpers.TestMetadata(
					fmt.Sprintf("load:throughput:hold:%d", userID), fmt.Sprintf("load-throughput-hold-%d", userID),
				)})
				if err != nil {
					errorsCh <- err
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("throughput Hold() error = %v", err)
		}
	}
	duration := time.Since(started)
	return duration, float64(commandCount) / duration.Seconds()
}

type creditPoolBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

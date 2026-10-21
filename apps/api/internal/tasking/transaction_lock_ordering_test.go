package tasking

// Cross-store transaction lock ordering gate.
//
// Every participant below shares one PostgreSQL database and joins a single
// caller-owned (or Control-guard borrowed) pgx.Tx. Inconsistent lock order
// across stores is a deadlock, so the call order pinned here is load-bearing:
// reorder any step and this test fails.
//
// The assertions are source-order checks scoped to one function body at a
// time (sliceBetween), so interface definitions and comments elsewhere in
// the file cannot satisfy them. Markers are exact call sites. A missing
// marker fails closed: update the code, docs/architecture/transaction-lock-ordering.md,
// and this test together.
//
// No database is required; this runs in the default unit suite.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func readOrderFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// sliceBetween returns the function body starting at the first occurrence of
// startAnchor and ending at the first occurrence of endAnchor after it (or
// EOF when endAnchor is empty).
func sliceBetween(t *testing.T, file, content, startAnchor, endAnchor string) string {
	t.Helper()
	start := strings.Index(content, startAnchor)
	if start < 0 {
		t.Fatalf("%s: start anchor %q not found (code moved; update doc+test)", file, startAnchor)
	}
	rest := content[start:]
	if endAnchor == "" {
		return rest
	}
	end := strings.Index(rest[len(startAnchor):], endAnchor)
	if end < 0 {
		t.Fatalf("%s: end anchor %q not found after %q (code moved; update doc+test)", file, endAnchor, startAnchor)
	}
	return rest[:len(startAnchor)+end]
}

// assertCallOrder requires every marker to exist exactly in the given order.
func assertCallOrder(t *testing.T, file, body string, markers []string) {
	t.Helper()
	prev := -1
	prevMarker := "<start>"
	for _, marker := range markers {
		idx := strings.Index(body, marker)
		if idx < 0 {
			t.Fatalf("%s: lock-order marker %q not found (code moved; update docs/architecture/transaction-lock-ordering.md and this test)", file, marker)
		}
		if idx <= prev {
			t.Fatalf("%s: lock order violated: %q must run before %q", file, prevMarker, marker)
		}
		prev, prevMarker = idx, marker
	}
}

func baselineMigration(t *testing.T) (string, string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "db", "migrations", "*baseline*.sql"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one baseline migration, got %v (err=%v)", matches, err)
	}
	sort.Strings(matches)
	return matches[0], readOrderFile(t, matches[0])
}

func TestTransactionLockOrdering(t *testing.T) {
	settle := readOrderFile(t, filepath.Join("workspacetasks", "settle_tx.go"))
	boards := readOrderFile(t, filepath.Join("..", "boards", "postgres_workspace.go"))
	tenancy := readOrderFile(t, filepath.Join("..", "tenancy", "infrastructure", "postgres", "store.go"))
	billing := readOrderFile(t, filepath.Join("..", "billing", "infrastructure", "postgres", "store.go"))
	baselineFile, baseline := baselineMigration(t)

	t.Run("settlement submit walks authorize claim quote hold task snapshot billing enqueue", func(t *testing.T) {
		body := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) createInTx", "func (s *Service) replayClaimedSubmission")
		assertCallOrder(t, "settle_tx.go:createInTx", body, []string{
			"s.acceptance.AuthorizeSubmission(ctx, tx, input)",
			"txClaims.ClaimSubmissionInTx(ctx, tx, claim)",
			"txAccounts.ResolveBillingAccountInTx(ctx, tx, claim.WorkspaceID)",
			"s.resolver.ResolveInTx(ctx, tx, kind, resolution)",
			"txQuoter.QuoteInTx(ctx, tx, kind, resolution, quantity)",
			"txHolder.HoldInTx(ctx, tx, billingAccountID, input.OwnerUserID",
			"txStore.CreateTaskInTx(ctx, tx, task)",
			"s.saveSnapshot(ctx, tx, created.ID, routingSnap)",
			"facts.CreateBillingFactsInTx(ctx, tx, created)",
			"txQueue.EnqueueInTx(ctx, tx, created)",
		})
	})

	t.Run("settlement terminal flips task before credits before billing settlement", func(t *testing.T) {
		complete := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) completeInTx", "func (s *Service) failInTx")
		assertCallOrder(t, "settle_tx.go:completeInTx", complete, []string{
			"txStore.CompleteTaskInTx(ctx, tx, workspaceID, taskID, outputs)",
			"txHolder.CaptureInTx(ctx, tx, completed.BillingAccountID, completed.OwnerUserID",
			"txHolder.ReleaseInTx(ctx, tx, completed.BillingAccountID, completed.OwnerUserID",
			"facts.SetBillingSettlementInTx(ctx, tx, taskID,",
		})
		failed := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) failInTx", "func (s *Service) cancelInTx")
		assertCallOrder(t, "settle_tx.go:failInTx", failed, []string{
			"txStore.FailTaskInTx(ctx, tx, workspaceID, taskID, errorMsg)",
			"txHolder.ReleaseInTx(ctx, tx, failed.BillingAccountID, failed.OwnerUserID",
			"facts.SetBillingSettlementInTx(ctx, tx, taskID,",
		})
	})

	t.Run("board writes lock workspace membership before board rows", func(t *testing.T) {
		hierarchy := sliceBetween(t, "postgres_workspace.go", boards,
			"func lockWritableBoardTx", "func (s *PostgresWorkspaceBoardStore) CreateBoard")
		assertCallOrder(t, "postgres_workspace.go:lockWritableBoardTx", hierarchy, []string{
			"lockWorkspaceMembersTx(ctx, q, workspacePublicID)",
			"q.LockWorkspaceBoardAccess(ctx,",
		})
		accept := sliceBetween(t, "postgres_workspace.go", boards,
			"func (s *PostgresWorkspaceBoardStore) ReplaceDocument", "func (s *PostgresWorkspaceBoardStore) ListRevisions")
		assertCallOrder(t, "postgres_workspace.go:ReplaceDocument", accept, []string{
			"lockWritableBoardTx(ctx, q, workspacePublicID, trimmedID, actorUserID)",
			"q.LockWorkspaceBoard(ctx,",
			"s.validateDocumentAssets(ctx, q,",
			"q.InsertBoardRevision(ctx,",
			"q.UpdateBoardDocument(ctx,",
			"q.DeleteBoardDocumentMediaRefs(ctx,",
			"q.UpsertBoardDocumentMediaRef(ctx,",
		})
	})

	t.Run("member departure locks workspace then boards in public id order before membership delete", func(t *testing.T) {
		departure := sliceBetween(t, "tenancy store.go", tenancy,
			"func (s *Store) RemoveWorkspaceMember", "func (s *Store) mutationContext")
		assertCallOrder(t, "tenancy store.go:RemoveWorkspaceMember", departure, []string{
			"q.LockWorkspaceMemberships(ctx,",
			"grants.RemoveWorkspaceMemberGrants(dbutil.ContextWithAmbientTx(ctx, tx), workspacePublicID, targetUserID)",
			"q.DeleteWorkspaceMembership(ctx,",
		})
		cleanup := sliceBetween(t, "postgres_workspace.go", boards,
			"func (s *PostgresWorkspaceBoardStore) RemoveWorkspaceMemberGrants", "")
		assertCallOrder(t, "postgres_workspace.go:RemoveWorkspaceMemberGrants", cleanup, []string{
			"q.ListWorkspaceMemberBoardGrants(ctx,",
			"q.LockWorkspaceMemberBoards(ctx,",
			"q.DeleteWorkspaceMemberBoardGrants(ctx,",
			"q.BumpWorkspaceMemberBoardsAccess(ctx,",
		})
	})

	t.Run("credits commands take advisory lock then wallet then accounts in id order", func(t *testing.T) {
		body := sliceBetween(t, baselineFile, baseline,
			"CREATE FUNCTION execute_credit_command_v2(", "$$ LANGUAGE plpgsql")
		assertCallOrder(t, baselineFile+":execute_credit_command_v2", body, []string{
			"pg_advisory_xact_lock(hashtextextended(p_idempotency_key",
			"FROM credit_wallets WHERE user_id = p_user_id FOR UPDATE",
			"ORDER BY accounts.id FOR UPDATE",
		})
	})

	t.Run("billing account commands keep the same lock order on the account wallet", func(t *testing.T) {
		fragment := readOrderFile(t, filepath.Join("..", "db", "schema", "fragments", "credits.sql"))
		body := sliceBetween(t, "fragments/credits.sql", fragment,
			"CREATE FUNCTION execute_credit_command_v3(", "$$ LANGUAGE plpgsql")
		assertCallOrder(t, "fragments/credits.sql:execute_credit_command_v3", body, []string{
			"pg_advisory_xact_lock(hashtextextended(p_idempotency_key",
			"FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE",
			"ORDER BY accounts.id FOR UPDATE",
		})
	})

	t.Run("billing quote stays lock free inside the settlement transaction", func(t *testing.T) {
		if !strings.Contains(billing, "func (s *Store) QuoteAtResolutionInTx") {
			t.Fatal("billing store.go: QuoteAtResolutionInTx seam missing (code moved; update doc+test)")
		}
		if strings.Contains(billing, "FOR UPDATE") {
			t.Fatal("billing store.go: Quote path must stay a lock-free SELECT; FOR UPDATE would serialize every settlement on the catalog")
		}
	})
}

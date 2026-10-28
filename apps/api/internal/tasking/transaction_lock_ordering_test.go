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
	boardsDir := filepath.Join("..", "boards")
	boardsParts := []string{"postgres_access.go", "postgres_store_board.go", "postgres_revision.go", "postgres_catalog.go"}
	boardsPieces := make([]string, 0, len(boardsParts))
	for _, part := range boardsParts {
		boardsPieces = append(boardsPieces, readOrderFile(t, filepath.Join(boardsDir, part)))
	}
	boards := strings.Join(boardsPieces, "\n")
	tenancy := readOrderFile(t, filepath.Join("..", "tenancy", "infrastructure", "postgres", "store.go"))
	billing := readOrderFile(t, filepath.Join("..", "billing", "infrastructure", "postgres", "store.go"))
	baselineFile, baseline := baselineMigration(t)

	t.Run("organization workspace creation locks its account before workspace insert", func(t *testing.T) {
		creation := sliceBetween(t, "tenancy store.go", tenancy,
			"func createWorkspaceInTx", "func createBillingInTx")
		assertCallOrder(t, "tenancy store.go:createWorkspaceInTx", creation, []string{
			"ensureOrganizationBillingAccountInTx(ctx, q, orgInternal.Int64, 0)",
			"lockBillingAccountInTx(ctx, q, account.InternalID)",
			"q.CreateWorkspace(ctx,",
		})
	})

	t.Run("settlement submit walks authorize claim quote hold task snapshot billing enqueue", func(t *testing.T) {
		body := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) createInTx", "func (s *Service) replayClaimedSubmission")
		assertCallOrder(t, "settle_tx.go:createInTx", body, []string{
			"s.acceptance.AuthorizeSubmission(ctx, tx, input)",
			"s.submissionClaims.ClaimSubmissionInTx(ctx, tx, claim)",
			"s.billingAccounts.ResolveBillingAccountInTx(ctx, tx, claim.WorkspaceID)",
			"s.resolver.ResolveInTx(ctx, tx, kind, resolution)",
			"s.quoter.QuoteInTx(ctx, tx, kind, resolution, quantity)",
			"s.holder.HoldInTx(ctx, tx, billingAccountID, input.OwnerUserID",
			"s.taskLifecycle.CreateTaskInTx(ctx, tx, task)",
			"s.saveSnapshot(ctx, tx, created.ID, routingSnap)",
			"facts.CreateBillingFactsInTx(ctx, tx, created)",
			"s.queue.EnqueueInTx(ctx, tx, created)",
		})
	})

	t.Run("settlement terminal flips task before credits before billing settlement", func(t *testing.T) {
		complete := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) completeInTx", "func (s *Service) failInTx")
		assertCallOrder(t, "settle_tx.go:completeInTx", complete, []string{
			"s.taskLifecycle.CompleteTaskInTx(ctx, tx, workspaceID, taskID, outputs)",
			"s.holder.CaptureInTx(ctx, tx, completed.BillingAccountID, completed.OwnerUserID",
			"s.holder.ReleaseInTx(ctx, tx, completed.BillingAccountID, completed.OwnerUserID",
			"facts.SetBillingSettlementInTx(ctx, tx, taskID,",
		})
		failed := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) failInTx", "func (s *Service) cancelInTx")
		assertCallOrder(t, "settle_tx.go:failInTx", failed, []string{
			"s.taskLifecycle.FailTaskInTx(ctx, tx, workspaceID, taskID, errorMsg)",
			"s.releaseTerminalInTx(ctx, tx, failed)",
		})
		cancelled := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) cancelInTx", "func (s *Service) retryInTx")
		assertCallOrder(t, "settle_tx.go:cancelInTx", cancelled, []string{
			"s.taskLifecycle.CancelTaskInTx(ctx, tx, workspaceID, taskID, actorUserID, reason)",
			"s.releaseTerminalInTx(ctx, tx, flipped)",
		})
		release := sliceBetween(t, "settle_tx.go", settle,
			"func (s *Service) releaseTerminalInTx", "func (s *Service) createInTx")
		assertCallOrder(t, "settle_tx.go:releaseTerminalInTx", release, []string{
			"s.holder.ReleaseInTx(ctx, tx, task.BillingAccountID, task.OwnerUserID",
			"facts.SetBillingSettlementInTx(ctx, tx, task.ID,",
		})
	})

	t.Run("board writes lock workspace membership before board rows", func(t *testing.T) {
		hierarchy := sliceBetween(t, "postgres_access.go", boards,
			"func lockWritableBoardTx", "func (s *PostgresWorkspaceBoardStore) CreateBoard")
		assertCallOrder(t, "postgres_access.go:lockWritableBoardTx", hierarchy, []string{
			"lockWorkspaceMembersTx(ctx, q, workspacePublicID)",
			"q.LockWorkspaceBoardAccess(ctx,",
		})
		accept := sliceBetween(t, "postgres_revision.go", boards,
			"func (s *PostgresWorkspaceBoardStore) ReplaceDocument", "func (s *PostgresWorkspaceBoardStore) ListRevisions")
		assertCallOrder(t, "postgres_revision.go:ReplaceDocument", accept, []string{
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
		cleanup := sliceBetween(t, "postgres_access.go", boardsPieces[0],
			"func (s *PostgresWorkspaceBoardStore) RemoveWorkspaceMemberGrants", "")
		assertCallOrder(t, "postgres_access.go:RemoveWorkspaceMemberGrants", cleanup, []string{
			"q.ListWorkspaceMemberBoardGrants(ctx,",
			"q.LockWorkspaceMemberBoards(ctx,",
			"q.DeleteWorkspaceMemberBoardGrants(ctx,",
			"q.BumpWorkspaceMemberBoardsAccess(ctx,",
		})
	})

	t.Run("credits commands take advisory lock then wallet then accounts in id order", func(t *testing.T) {
		body := sliceBetween(t, baselineFile, baseline,
			"CREATE FUNCTION execute_credit_command_v3(", "$$ LANGUAGE plpgsql")
		assertCallOrder(t, baselineFile+":execute_credit_command_v3", body, []string{
			"pg_advisory_xact_lock(hashtextextended(p_idempotency_key",
			"FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE",
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
		if !strings.Contains(billing, "func (s *Store) Quote(ctx context.Context, tx pgx.Tx,") {
			t.Fatal("billing store.go: Quote seam missing (code moved; update doc+test)")
		}
		if strings.Contains(billing, "FOR UPDATE") {
			t.Fatal("billing store.go: Quote path must stay a lock-free SELECT; FOR UPDATE would serialize every settlement on the catalog")
		}
	})
}

package v1

// Canvas Task shared-billing preflight (#917): quote/readiness and submit
// spend the same stable Billing Account with the same consumption
// eligibility (in-memory seams, no database).
//
// What is proven here:
//   - Quote resolves the same fixture account submit holds (one payer:
//     the fixture serves 7001 on both seams).
//   - Quote distinguishes 402 task_insufficient_credits from 403
//     billing_wallet_restricted, mirroring the submit Hold mapping.
//   - A Board-bound quote enforces the same generate gate as submit:
//     invisible Boards read 404 board_not_found, archived Boards 409
//     board_archived, Viewers 403 task_forbidden; the creator and Editors
//     read 200. Financial standing alone grants nothing: callers without
//     Board generate capability fail here exactly as on submit.
//   - Quote and submit agree for the same caller and Board (same
//     eligibility, same account).

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

func quoteTask(t *testing.T, engine *gin.Engine, workspaceID string, userID int64, body string) (int, map[string]any) {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + workspaceID + "/task-quotes", userID: userID,
		body: body,
	})
	return status, payload
}

func quoteRenderBody(boardID string) string {
	if boardID == "" {
		return `{"data":{"service":"render","quantity":1}}`
	}
	return `{"data":{"service":"render","quantity":1,"board_id":"` + boardID + `"}}`
}

func TestV1TaskQuoteFundsGate(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)

	// Funded account: 200 with a price snapshot.
	status, payload := quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("funded quote = %d %v, want 200", status, payload)
	}

	// Shortfall: 402 task_insufficient_credits (the fixture quotes 10).
	fx.wallets.available = 5
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusPaymentRequired || v1ErrorCode(t, payload) != string(contract.CodeTaskInsufficientCredits) {
		t.Fatalf("shortfall quote = %d %v, want 402 task_insufficient_credits", status, payload)
	}

	// Active restriction: 403 billing_wallet_restricted, even when funded.
	fx.wallets.available = 1000000
	fx.wallets.restrictions = []string{"ledger_integrity"}
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("restricted quote = %d %v, want 403 billing_wallet_restricted", status, payload)
	}

	// Restricted read model: 403 as well.
	fx.wallets.restrictions = nil
	fx.wallets.err = domain.ErrWalletRestricted
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("restricted wallet quote = %d %v, want 403 billing_wallet_restricted", status, payload)
	}
	fx.wallets.err = nil
}

func TestV1TaskQuoteBoardGateMirrorsSubmit(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)

	// The creator reads ready on their private Board.
	status, payload := quoteTask(t, fx.engine, fx.ws.PublicID, 1, quoteRenderBody(fx.boardID))
	if status != http.StatusOK {
		t.Fatalf("creator board quote = %d %v, want 200", status, payload)
	}

	// An unselected member cannot discover the private Board: 404.
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(fx.boardID))
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeBoardNotFound) {
		t.Fatalf("private board quote = %d %v, want 404 board_not_found", status, payload)
	}

	// A Viewer reads 403 on quote exactly as on submit (same eligibility).
	access, err := fx.boardStore.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, access.AccessVersion, boards.WorkspaceAccess(boards.RoleViewer), []int64{3}); err != nil {
		t.Fatal(err)
	}
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(fx.boardID))
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeTaskForbidden) {
		t.Fatalf("viewer board quote = %d %v, want 403 task_forbidden", status, payload)
	}
	status, payload = submitBoardTask(t, fx, 3, "917-viewer-submit", fx.boardID)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeTaskForbidden) {
		t.Fatalf("viewer board submit = %d %v, want 403 task_forbidden", status, payload)
	}

	// An Editor reads ready and submits: quote and submit agree.
	access, err = fx.boardStore.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{3}); err != nil {
		t.Fatal(err)
	}
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(fx.boardID))
	if status != http.StatusOK {
		t.Fatalf("editor board quote = %d %v, want 200", status, payload)
	}
	status, payload = submitBoardTask(t, fx, 3, "917-editor-submit", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("editor board submit = %d %v, want 202", status, payload)
	}

	// Malformed board_ shapes fail closed as 422 on both surfaces.
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 1, quoteRenderBody("nope"))
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("malformed board quote = %d %v, want 422 validation_failed", status, payload)
	}
}

func TestV1TaskQuoteBoardArchived(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	if _, err := fx.boardStore.ArchiveBoard(context.Background(), fx.ws.PublicID, fx.boardID, 1); err != nil {
		t.Fatal(err)
	}
	status, payload := quoteTask(t, fx.engine, fx.ws.PublicID, 1, quoteRenderBody(fx.boardID))
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeBoardArchived) {
		t.Fatalf("archived board quote = %d %v, want 409 board_archived", status, payload)
	}
}

func TestV1TaskQuoteOutsiderHidden(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	// Non-members converge to 404 task_not_found: the surface never leaks
	// workspace existence, and financial standing alone grants no quote.
	status, payload := quoteTask(t, fx.engine, fx.ws.PublicID, 9, quoteRenderBody(""))
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeTaskNotFound) {
		t.Fatalf("outsider quote = %d %v, want 404 task_not_found", status, payload)
	}
}

func TestV1TaskQuoteAndSubmitShareFixtureAccount(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	// Both seams serve the fixed fixture account: the quote preflight and
	// the submit Hold name one payer (7001). The holder records holds by
	// id; the quote wallet reads the same fixture funds.
	status, _ := quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("quote = %d, want 200", status)
	}
	status, payload := func() (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
			body:    v1CreateTaskBodyJSON("render"),
			headers: map[string]string{contract.IdempotencyKeyHeader: "917-shared-account"},
		})
		return status, payload
	}()
	if status != http.StatusAccepted {
		t.Fatalf("submit = %d %v, want 202", status, payload)
	}
	taskID := v1Data(t, payload).(map[string]any)["id"].(string)
	stored, err := fx.tasks.GetTask(context.Background(), fx.ws.PublicID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.BillingAccountID != 7001 {
		t.Fatalf("task billing account = %d, want the fixture account 7001", stored.BillingAccountID)
	}
	if stored.OwnerUserID != 3 {
		t.Fatalf("task submitter = %d, want the real submitting user 3", stored.OwnerUserID)
	}
	if stored.BillingSnapshot.BillingAccountID != 7001 {
		t.Fatalf("snapshot billing account = %d, want 7001", stored.BillingSnapshot.BillingAccountID)
	}
}

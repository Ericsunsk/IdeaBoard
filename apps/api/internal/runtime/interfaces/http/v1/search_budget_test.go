package v1

import (
	"fmt"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/workspacebudgets"
	"net/http"
	"testing"
)

// Vision budget/quota mapping (issues #945/#946): the legacy
// vision-queries path has no VisionBillingService settlement of its own,
// so budget/quota failures surface through the Quoter/Settler seams. They
// must converge to the Task-side tricolor 403s
// (workspace_budget_exhausted / member_quota_exhausted), never the 503
// default or billing_wallet_restricted.
func TestV1VisionQueryBudgetQuotaMapping(t *testing.T) {
	build := func(quoter *fakeLibraryQuoter, billing *fakeLibraryBilling) *v1SearchFixture {
		return newV1SearchFixture(t, ContractV1SearchDeps{
			Backend: &fakeSearchBackend{}, Library: nil, Quoter: quoter, Settler: billing,
			Extractor: &fakeVisionExtractor{query: "x"},
		})
	}
	post := func(fx *v1SearchFixture, key string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: fx.visionPath(),
			userID: fx.owner, body: `{"data":{"user_text":"cafe"}}`,
			headers: map[string]string{contract.IdempotencyKeyHeader: key}})
		return status, payload
	}

	// Quoter-side failures take the multi-%w quote wrap (alongside
	// errV1BillingQuoteUnavailable); the budget/quota cause must still win
	// over the transient 503 branch.
	quoteCases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"budget exhausted", workspacebudgets.ErrBudgetExhausted, string(contract.CodeWorkspaceBudgetExhausted)},
		{"quota exhausted", memberquotas.ErrQuotaExhausted, string(contract.CodeMemberQuotaExhausted)},
	}
	for i, tc := range quoteCases {
		fx := build(&fakeLibraryQuoter{err: tc.err}, &fakeLibraryBilling{})
		status, payload := post(fx, fmt.Sprintf("vq-bq-quote-%d", i))
		if status != http.StatusForbidden || v1ErrorCode(t, payload) != tc.wantCode {
			t.Fatalf("quote %s = %d %v, want 403 %s", tc.name, status, payload, tc.wantCode)
		}
	}

	// Hold-side failures propagate raw through the idempotency scope and
	// must take the same billing branch.
	holdCases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"budget exhausted", workspacebudgets.ErrBudgetExhausted, string(contract.CodeWorkspaceBudgetExhausted)},
		{"quota exhausted", memberquotas.ErrQuotaExhausted, string(contract.CodeMemberQuotaExhausted)},
	}
	for i, tc := range holdCases {
		fx := build(&fakeLibraryQuoter{amount: 3}, &fakeLibraryBilling{holdErr: tc.err})
		status, payload := post(fx, fmt.Sprintf("vq-bq-hold-%d", i))
		if status != http.StatusForbidden || v1ErrorCode(t, payload) != tc.wantCode {
			t.Fatalf("hold %s = %d %v, want 403 %s", tc.name, status, payload, tc.wantCode)
		}
	}
}

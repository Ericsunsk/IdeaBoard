package control

// Admin explicit-Billing-Account contract tests (#923): account list/detail,
// ba_-addressed wallet/ledger reads, XOR-addressed fund commands, and manual
// restriction create/release. Fund writes stay on the typed Commands seam
// (asserted through the fake), the Ledger Actor stays the platform operator,
// and auto-restriction semantics are pinned at the domain level, not here.

import (
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

func newAccountControlHarness(t *testing.T) (*controlHarness, *fakeControlCredits, *fakeControlRestrictions) {
	t.Helper()
	credits := &fakeControlCredits{}
	restrictions := &fakeControlRestrictions{}
	h := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.Credits = credits
		deps.Restrictions = restrictions
	})
	return h, credits, restrictions
}

func adminCall(call controlCall) controlCall {
	call.userID = controlOperatorID
	call.role = "admin"
	return call
}

func TestControlAccountGrantFlow(t *testing.T) {
	h, credits, _ := newAccountControlHarness(t)

	t.Run("ba addressed grant writes the explicit account with operator actor", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants",
			body:    `{"billing_account_id":"ba_orgpool1","amount_credits":10,"business_reference_id":"ops-1"}`,
			headers: controlMutationHeaders("account-grant-1"),
		}))
		if status != http.StatusOK {
			t.Fatalf("grant status = %d, body=%v", status, payload)
		}
		data := controlData(t, payload).(map[string]any)
		wallet := data["wallet"].(map[string]any)
		if wallet["billing_account_id"] != "ba_orgpool1" {
			t.Fatalf("wallet = %v, want ba_orgpool1", wallet)
		}
		if _, hasUser := wallet["user_id"]; hasUser {
			t.Fatalf("account grant wallet must not synthesize user linkage: %v", wallet)
		}
		credits.mu.Lock()
		defer credits.mu.Unlock()
		if credits.lastBillingAccountID != 7001 {
			t.Fatalf("command billing account = %d, want 7001", credits.lastBillingAccountID)
		}
		if credits.lastMeta.Actor.Type != domain.ActorAdmin {
			t.Fatalf("command actor = %+v, want admin", credits.lastMeta.Actor)
		}
		records := h.sink.Records()
		last := records[len(records)-1]
		if last.TargetType != "billing_account" || last.TargetID != "ba_orgpool1" {
			t.Fatalf("audit target = %q/%q, want billing_account/ba_orgpool1", last.TargetType, last.TargetID)
		}
	})

	t.Run("user addressed grant keeps personal linkage", func(t *testing.T) {
		targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants",
			body:    fmt.Sprintf(`{"user_id":%q,"amount_credits":10,"business_reference_id":"ops-2"}`, targetUser),
			headers: controlMutationHeaders("account-grant-2"),
		}))
		if status != http.StatusOK {
			t.Fatalf("grant status = %d, body=%v", status, payload)
		}
		data := controlData(t, payload).(map[string]any)
		wallet := data["wallet"].(map[string]any)
		if wallet["user_id"] != targetUser {
			t.Fatalf("wallet = %v, want user linkage %s", wallet, targetUser)
		}
	})

	t.Run("ambiguous and missing addressing fail closed", func(t *testing.T) {
		targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
		for name, body := range map[string]string{
			"both":      fmt.Sprintf(`{"user_id":%q,"billing_account_id":"ba_orgpool1","amount_credits":10,"business_reference_id":"ops-x"}`, targetUser),
			"neither":   `{"amount_credits":10,"business_reference_id":"ops-x"}`,
			"malformed": `{"billing_account_id":"nope","amount_credits":10,"business_reference_id":"ops-x"}`,
		} {
			status, _, payload := h.serve(adminCall(controlCall{
				method: http.MethodPost, path: "/api/control/v1/credits/grants",
				body: body, headers: controlMutationHeaders("account-grant-xor-" + name),
			}))
			if status != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want 400 (body=%v)", name, status, payload)
			}
			if code := controlErrorCode(t, payload); code != string(contract.CodeBadRequest) {
				t.Fatalf("%s: code = %q, want bad_request", name, code)
			}
		}
	})

	t.Run("deduction refund correction accept ba addressing", func(t *testing.T) {
		calls := []controlCall{
			{method: http.MethodPost, path: "/api/control/v1/credits/deductions",
				body: `{"billing_account_id":"ba_orgpool1","amount_credits":5,"business_reference_id":"ops-d"}`},
			{method: http.MethodPost, path: "/api/control/v1/credits/refunds",
				body: `{"billing_account_id":"ba_orgpool1","amount_credits":5,"business_reference_id":"ops-r","charge_transaction_id":"ctxn_charge_1"}`},
			{method: http.MethodPost, path: "/api/control/v1/credits/corrections",
				body: `{"billing_account_id":"ba_orgpool1","amount_credits":5,"business_reference_id":"ops-c","original_transaction_id":"ctxn_orig_1"}`},
		}
		for i, call := range calls {
			call = adminCall(call)
			call.headers = controlMutationHeaders(fmt.Sprintf("account-mutate-%d", i))
			status, _, payload := h.serve(call)
			if status != http.StatusOK {
				t.Fatalf("%s status = %d, body=%v", call.path, status, payload)
			}
		}
	})

	t.Run("linked refund and correction still require their link", func(t *testing.T) {
		for path, body := range map[string]string{
			"/api/control/v1/credits/refunds":     `{"billing_account_id":"ba_orgpool1","amount_credits":5,"business_reference_id":"ops-r2"}`,
			"/api/control/v1/credits/corrections": `{"billing_account_id":"ba_orgpool1","amount_credits":5,"business_reference_id":"ops-c2"}`,
		} {
			status, _, payload := h.serve(adminCall(controlCall{
				method: http.MethodPost, path: path,
				body: body, headers: controlMutationHeaders("account-link-missing"),
			}))
			if status != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want 400 (body=%v)", path, status, payload)
			}
		}
	})
}

func TestControlAccountReads(t *testing.T) {
	h, _, _ := newAccountControlHarness(t)

	t.Run("account wallet read is ba addressed without user linkage", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodGet, path: "/api/control/v1/credits/accounts/ba_orgpool1",
		}))
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		wallet := controlData(t, payload).(map[string]any)
		if wallet["billing_account_id"] != "ba_orgpool1" {
			t.Fatalf("wallet = %v, want ba_orgpool1", wallet)
		}
		if _, hasUser := wallet["user_id"]; hasUser {
			t.Fatalf("account wallet must not synthesize user linkage: %v", wallet)
		}
	})

	t.Run("account wallet rejects malformed ba", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodGet, path: "/api/control/v1/credits/accounts/nope",
		}))
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%v)", status, payload)
		}
	})

	t.Run("account ledger read pages", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodGet, path: "/api/control/v1/credits/accounts/ba_orgpool1/ledger",
		}))
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
	})

	t.Run("account list pages without workspace", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodGet, path: "/api/control/v1/billing/accounts?owner_scope=all",
		}))
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
	})

	t.Run("account detail resolves linkage", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodGet, path: "/api/control/v1/billing/accounts/ba_orgpool1",
		}))
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		detail := controlData(t, payload).(map[string]any)
		if detail["billing_account_id"] != "ba_orgpool1" {
			t.Fatalf("detail = %v, want ba_orgpool1", detail)
		}
	})

	t.Run("non-admin denied on account surface", func(t *testing.T) {
		for _, call := range []controlCall{
			{method: http.MethodGet, path: "/api/control/v1/credits/accounts/ba_orgpool1"},
			{method: http.MethodGet, path: "/api/control/v1/billing/accounts"},
			{method: http.MethodGet, path: "/api/control/v1/billing/accounts/ba_orgpool1"},
		} {
			call.userID = controlUserAID
			call.role = "user"
			status, _, _ := h.serve(call)
			if status != http.StatusForbidden {
				t.Fatalf("%s %s status = %d, want 403", call.method, call.path, status)
			}
		}
	})
}

func TestControlRestrictionOps(t *testing.T) {
	h, _, restrictions := newAccountControlHarness(t)

	t.Run("restrict binds the explicit account with admin actor", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/wallet-restrictions",
			body: `{"billing_account_id":"ba_orgpool1","type":"spending","source":"control_review",` +
				`"related_object_type":"finding","related_object_id":"42","reason":"manual review"}`,
			headers: controlMutationHeaders("restrict-1"),
		}))
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		restrictions.mu.Lock()
		defer restrictions.mu.Unlock()
		if restrictions.restrictCalls != 1 {
			t.Fatalf("restrict calls = %d, want 1", restrictions.restrictCalls)
		}
		if restrictions.lastRestrict.BillingAccountID != 7001 {
			t.Fatalf("restriction account = %d, want 7001", restrictions.lastRestrict.BillingAccountID)
		}
		if restrictions.lastRestrict.Actor.Type != domain.ActorAdmin {
			t.Fatalf("restriction actor = %+v, want admin", restrictions.lastRestrict.Actor)
		}
		records := h.sink.Records()
		last := records[len(records)-1]
		if last.TargetType != "billing_account" || last.TargetID != "ba_orgpool1" {
			t.Fatalf("audit target = %q/%q, want billing_account/ba_orgpool1", last.TargetType, last.TargetID)
		}
	})

	t.Run("release addresses exactly one restriction", func(t *testing.T) {
		status, _, payload := h.serve(adminCall(controlCall{
			method:  http.MethodPost,
			path:    "/api/control/v1/credits/wallet-restrictions/00000000-0000-4000-8000-000000000001/release",
			body:    `{"reason":"resolved"}`,
			headers: controlMutationHeaders("release-1"),
		}))
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		data := controlData(t, payload).(map[string]any)
		if data["restriction_id"] != "00000000-0000-4000-8000-000000000001" {
			t.Fatalf("result = %v, want the addressed restriction", data)
		}
	})

	t.Run("release of unknown restriction is 404 without new policy", func(t *testing.T) {
		failing := &fakeControlRestrictions{fail: domain.ErrWalletRestrictionNotFound}
		h3 := newControlHarness(t, func(deps *ContractControlDeps) {
			deps.Restrictions = failing
		})
		status, _, payload := h3.serve(adminCall(controlCall{
			method:  http.MethodPost,
			path:    "/api/control/v1/credits/wallet-restrictions/00000000-0000-4000-8000-000000000009/release",
			body:    `{"reason":"resolved"}`,
			headers: controlMutationHeaders("release-404"),
		}))
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body=%v)", status, payload)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlNotFound) {
			t.Fatalf("code = %q, want control_not_found", code)
		}
	})

	t.Run("unwired restrictions stay fail-closed", func(t *testing.T) {
		plain := newControlHarness(t, nil)
		status, _, _ := plain.serve(adminCall(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/wallet-restrictions",
			body: `{"billing_account_id":"ba_orgpool1","type":"spending","source":"control_review",` +
				`"related_object_type":"finding","related_object_id":"42"}`,
			headers: controlMutationHeaders("restrict-unwired"),
		}))
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", status)
		}
	})

	t.Run("mutations require idempotency key", func(t *testing.T) {
		status, _, _ := h.serve(adminCall(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/wallet-restrictions",
			body: `{"billing_account_id":"ba_orgpool1","type":"spending","source":"control_review","related_object_type":"finding","related_object_id":"42"}`,
		}))
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
}

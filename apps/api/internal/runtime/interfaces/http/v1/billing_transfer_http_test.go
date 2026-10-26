package v1

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

// Focused HTTP coverage for #948 invite-style handover: owner-only
// initiate, recipient accept/reject, initiator cancel, idempotent replay,
// and billing detach without deleting funds.

func handoverHTTPFixture(t *testing.T, ctx context.Context, owner, recipient int64) (*billingTestFixture, *tenancy.Workspace) {
	t.Helper()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "handover-http", owner)
	fx.store.SeedWorkspaceMember(ws.PublicID, recipient, tenancy.RoleMember)
	for _, id := range []int64{owner, recipient} {
		fx.dir.ensure(id)
	}
	return fx, ws
}

func TestV1TransferInviteRequiresOwner(t *testing.T) {
	ctx := context.Background()
	fx, ws := handoverHTTPFixture(t, ctx, 7, 8)
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/transfer-invites"
	usr8 := tenancy.FormatUserPublicID(fx.dir.ensure(8))
	usr7 := tenancy.FormatUserPublicID(fx.dir.ensure(7))

	// Non-owner (recipient without billing grant) cannot initiate.
	// The fixture default grants every caller Owner, so deny recipient first.
	fx.accounts.deny = map[int64]bool{8: true}
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 8,
		body:    `{"data":{"new_owner_user_id":` + quoteJSON(usr7) + `}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-require-owner-1"},
	})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("non-owner initiate = %d %v, want 403 billing_account_forbidden", status, payload)
	}
	delete(fx.accounts.deny, 8)

	// Owner initiates.
	status, _, created := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body:    `{"data":{"new_owner_user_id":` + quoteJSON(usr8) + `}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-require-owner-2"},
	})
	if status != http.StatusCreated {
		t.Fatalf("owner initiate = %d %v, want 201", status, created)
	}
	data := v1Data(t, created).(map[string]any)
	if data["status"] != tenancy.BillingTransferPending || data["to_user_id"] != usr8 {
		t.Fatalf("invite = %v, want pending to %s", data, usr8)
	}
	// Missing key fails closed.
	status, _, missing := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body: `{"data":{"new_owner_user_id":` + quoteJSON(usr8) + `}}`,
	})
	if status != http.StatusBadRequest || v1ErrorCode(t, missing) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("initiate without key = %d %v, want 400 idempotency_key_required", status, missing)
	}
}

func TestV1TransferInviteAcceptRejectCancel(t *testing.T) {
	ctx := context.Background()
	fx, ws := handoverHTTPFixture(t, ctx, 7, 8)
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/transfer-invites"
	usr8 := tenancy.FormatUserPublicID(fx.dir.ensure(8))

	status, _, created := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body:    `{"data":{"new_owner_user_id":` + quoteJSON(usr8) + `}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-flow-1"},
	})
	if status != http.StatusCreated {
		t.Fatalf("initiate = %d %v, want 201", status, created)
	}
	inviteID := v1Data(t, created).(map[string]any)["public_id"].(string)

	// Only the recipient may accept.
	status, _, forbidden := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base + "/" + inviteID + "/accept", userID: 7,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-accept-1"},
	})
	if status != http.StatusForbidden || v1ErrorCode(t, forbidden) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("initiator accept = %d %v, want 403 billing_account_forbidden", status, forbidden)
	}
	// Recipient rejects first (membership unchanged), then a fresh invite accepts.
	status, _, rejected := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base + "/" + inviteID + "/reject", userID: 8,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-reject-1"},
	})
	if status != http.StatusOK || v1Data(t, rejected).(map[string]any)["status"] != tenancy.BillingTransferRejected {
		t.Fatalf("reject = %d %v, want 200 rejected", status, rejected)
	}
	// Terminal invites reject further transitions.
	status, _, conflict := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base + "/" + inviteID + "/accept", userID: 8,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-accept-2"},
	})
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeBillingAccountConflict) {
		t.Fatalf("accept-after-reject = %d %v, want 409 billing_account_conflict", status, conflict)
	}

	status, _, created2 := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body:    `{"data":{"new_owner_user_id":` + quoteJSON(usr8) + `}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-flow-2"},
	})
	if status != http.StatusCreated {
		t.Fatalf("second initiate = %d %v, want 201", status, created2)
	}
	invite2 := v1Data(t, created2).(map[string]any)["public_id"].(string)
	status, _, accepted := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base + "/" + invite2 + "/accept", userID: 8,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-accept-3"},
	})
	if status != http.StatusOK || v1Data(t, accepted).(map[string]any)["status"] != tenancy.BillingTransferAccepted {
		t.Fatalf("accept = %d %v, want 200 accepted", status, accepted)
	}
}

func TestV1TransferInviteIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	fx, ws := handoverHTTPFixture(t, ctx, 7, 8)
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/transfer-invites"
	usr8 := tenancy.FormatUserPublicID(fx.dir.ensure(8))

	body := `{"data":{"new_owner_user_id":` + quoteJSON(usr8) + `}}`
	status, _, first := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7, body: body,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-idem-1"},
	})
	if status != http.StatusCreated {
		t.Fatalf("initiate = %d %v, want 201", status, first)
	}
	firstID := v1Data(t, first).(map[string]any)["public_id"].(string)
	status, _, replayed := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7, body: body,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-idem-1"},
	})
	if status != http.StatusCreated || v1Data(t, replayed).(map[string]any)["public_id"] != firstID {
		t.Fatalf("replay = %d %v, want same invite %s", status, replayed, firstID)
	}
	// Same key with a different recipient conflicts instead of replaying.
	usr9 := tenancy.FormatUserPublicID(fx.dir.ensure(9))
	status, _, conflict := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body:    `{"data":{"new_owner_user_id":` + quoteJSON(usr9) + `}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-idem-1"},
	})
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeBillingAccountConflict) {
		t.Fatalf("divergent replay = %d %v, want 409 billing_account_conflict", status, conflict)
	}
}

func TestV1BillingDetachKeepsFunds(t *testing.T) {
	ctx := context.Background()
	fx, ws := handoverHTTPFixture(t, ctx, 7, 8)
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/members"
	usr8 := tenancy.FormatUserPublicID(fx.dir.ensure(8))

	// Grant recipient as admin, then detach (no key: idempotent detach).
	status, _, added := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body:    `{"data":{"user_id":` + quoteJSON(usr8) + `,"role":"admin"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "handover-detach-grant-1"},
	})
	if status != http.StatusCreated {
		t.Fatalf("grant admin = %d %v, want 201", status, added)
	}
	status, _, removed := doV1(t, engine, v1Call{
		method: http.MethodDelete, path: base + "/" + usr8, userID: 8,
	})
	if status != http.StatusOK {
		t.Fatalf("self detach = %d %v, want 200", status, removed)
	}
	// Last Owner cannot detach (never orphan funds management).
	usr7 := tenancy.FormatUserPublicID(fx.dir.ensure(7))
	status, _, last := doV1(t, engine, v1Call{
		method: http.MethodDelete, path: base + "/" + usr7, userID: 7,
	})
	if status != http.StatusForbidden || v1ErrorCode(t, last) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("last-owner detach = %d %v, want 403 billing_account_forbidden", status, last)
	}
}

func TestV1TransferInviteVisibilityIncludesOnlyAddressedRecipient(t *testing.T) {
	ctx := context.Background()
	fx, ws := handoverHTTPFixture(t, ctx, 7, 8)
	fx.store.SeedWorkspaceMember(ws.PublicID, 9, tenancy.RoleMember)
	fx.dir.ensure(9)
	identity, err := fx.accounts.ResolveFunds(ctx, ws.PublicID)
	account := identity.ToBillingAccount()
	if err != nil {
		t.Fatal(err)
	}
	invited, err := fx.accounts.InitiateBillingTransferInvite(ctx, account.InternalID, 7, 8, "visible-recipient")
	if err != nil {
		t.Fatal(err)
	}
	fx.accounts.deny = map[int64]bool{8: true, 9: true}
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/transfer-invites"
	for _, tc := range []struct {
		actor int64
		count int
	}{{7, 1}, {8, 1}, {9, 0}} {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: base, userID: tc.actor})
		if status != http.StatusOK {
			t.Fatalf("actor %d list = %d %v", tc.actor, status, payload)
		}
		rows := v1Data(t, payload).([]any)
		if len(rows) != tc.count {
			t.Fatalf("actor %d saw %d invites, want %d", tc.actor, len(rows), tc.count)
		}
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: base + "/" + invited.PublicID, userID: tc.actor})
		want := http.StatusOK
		if tc.actor == 9 {
			want = http.StatusNotFound
		}
		if status != want {
			t.Fatalf("actor %d detail = %d %v, want %d", tc.actor, status, payload, want)
		}
	}
}

func TestV1TransferInviteCrossAccountDecisionsHaveNoEffects(t *testing.T) {
	for _, decision := range []string{"accept", "reject", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			ctx := context.Background()
			fx, source := handoverHTTPFixture(t, ctx, 7, 8)
			actor := int64(8)
			if decision == "cancel" {
				actor = 7
			}
			// The unrelated workspace lives in a third holder's scope
			// (ADR-0038: one Personal survivor per holder), so its payer
			// account can never see the source invite; the actor reads it as
			// an explicit workspace member.
			unrelated := v1WorkspaceFixture(t, ctx, fx.store, "unrelated", 9)
			fx.store.SeedWorkspaceMember(unrelated.PublicID, actor, tenancy.RoleMember)
			identity, err := fx.accounts.ResolveFunds(ctx, source.PublicID)
			account := identity.ToBillingAccount()
			if err != nil {
				t.Fatal(err)
			}
			invited, err := fx.accounts.InitiateBillingTransferInvite(ctx, account.InternalID, 7, 8, "wrong-account-create")
			if err != nil {
				t.Fatal(err)
			}
			status, _, payload := doV1(t, fx.engine(), v1Call{
				method: http.MethodPost,
				path:   "/api/v1/workspaces/" + unrelated.PublicID + "/billing-account/transfer-invites/" + invited.PublicID + "/" + decision,
				userID: actor, headers: map[string]string{contract.IdempotencyKeyHeader: "wrong-account-" + decision},
			})
			if status != http.StatusNotFound {
				t.Fatalf("decision = %d %v", status, payload)
			}
			current, err := fx.accounts.GetBillingTransferInvite(ctx, invited.PublicID)
			if err != nil || current.Status != tenancy.BillingTransferPending {
				t.Fatalf("404 changed invite: %+v %v", current, err)
			}
		})
	}
}

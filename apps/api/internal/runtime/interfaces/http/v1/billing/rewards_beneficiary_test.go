package billing

// Reward beneficiary account tests (issue #921).
//
// Fresh-DB friendly: in-memory tenancy store + fakes only, no testdb.
// The requesting workspace is the authZ scope only: the Grant beneficiary
// stays the caller's initial personal Billing Account, the edge names its
// stable ba_ identity, and the same key + same code replays across
// workspaces instead of conflicting or double-granting.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/rewards/ports"
)

type beneficiaryRewards struct {
	calls      int
	lastUserID int64
	results    map[int64]ports.RedeemResult
}

func (f *beneficiaryRewards) Redeem(ctx context.Context, cmd ports.RedeemCommand) (ports.RedeemResult, error) {
	f.calls++
	f.lastUserID = cmd.UserID
	if existing, ok := f.results[cmd.UserID]; ok {
		replayed := existing
		replayed.Replayed = true
		return replayed, nil
	}
	if f.results == nil {
		f.results = make(map[int64]ports.RedeemResult)
	}
	result := ports.RedeemResult{
		Redemption: ports.Redemption{
			ID: 900 + cmd.UserID, RewardCodeID: 700, UserID: cmd.UserID,
			CreditAmount: 60, CreditTransactionID: "tx-beneficiary",
			AvailableBalanceAfter: 1060, RedeemedAt: billingTestNow,
			RewardCodeDisplayName: "Beneficiary reward", MaskedCodeHint: "BENE••••2026",
		},
		CreditedAmount:   60,
		AvailableBalance: 1060,
		BillingAccountID: 8100 + cmd.UserID,
	}
	f.results[cmd.UserID] = result
	result.HistoryItem = ports.RedemptionHistoryItem{
		ID: result.Redemption.ID, RewardCodeDisplayName: result.Redemption.RewardCodeDisplayName,
		CreditedAmount: result.CreditedAmount, MaskedCodeHint: result.Redemption.MaskedCodeHint,
		RedeemedAt: result.Redemption.RedeemedAt,
	}
	f.results[cmd.UserID] = result
	return result, nil
}

func (f *beneficiaryRewards) ListRedemptions(ctx context.Context, input ports.ListRedemptionsInput) (ports.RedemptionHistoryPage, error) {
	return ports.RedemptionHistoryPage{}, nil
}

func beneficiaryFixture(t *testing.T) (*gapsTestFixture, *beneficiaryRewards) {
	t.Helper()
	fx := newGapsTestFixture()
	rewards := &beneficiaryRewards{}
	fx.deps.Rewards = rewards
	return fx, rewards
}

func TestV1RewardBeneficiaryStaysPersonalAcrossWorkspaces(t *testing.T) {
	fx, rewards := beneficiaryFixture(t)
	firstWS := gapsWorkspace(t, fx)
	secondWS := v1WorkspaceFixture(t, context.Background(), fx.store, "beneficiary-second", 11).PublicID
	engine := fx.engine()

	status, _, first := doV1(t, engine, gapsRedeemCall(firstWS, "BENE-2026", "beneficiary-key-1", 11))
	if status != http.StatusCreated {
		t.Fatalf("first redeem status = %d (%v)", status, first)
	}
	firstData := v1Data(t, first).(map[string]any)
	if firstData["billing_account_id"] != "ba_beneficiary_8111" {
		t.Fatalf("beneficiary = %v, want caller personal ba_beneficiary_8111", firstData)
	}
	if firstData["available_balance"] != float64(1060) {
		t.Fatalf("balance = %v, want beneficiary balance 1060", firstData)
	}
	if rewards.lastUserID != 11 {
		t.Fatalf("redeem user = %d, want caller 11", rewards.lastUserID)
	}

	// Same key + same code from a second workspace replays the first
	// redemption: one beneficiary, no second Grant, no conflict.
	before := rewards.calls
	status, _, replayed := doV1(t, engine, gapsRedeemCall(secondWS, "BENE-2026", "beneficiary-key-1", 11))
	if status != http.StatusCreated {
		t.Fatalf("cross-workspace replay status = %d (%v)", status, replayed)
	}
	replayData := v1Data(t, replayed).(map[string]any)
	if replayData["redemption"].(map[string]any)["id"] != firstData["redemption"].(map[string]any)["id"] {
		t.Fatalf("replay = %v, want same redemption id", replayed)
	}
	if replayData["replayed"] != true || replayData["billing_account_id"] != "ba_beneficiary_8111" {
		t.Fatalf("replay = %v, want replayed with the same beneficiary", replayed)
	}
	if rewards.calls != before {
		t.Fatal("cross-workspace same key + same code must replay, not re-execute")
	}

	// Same key + different code still conflicts (no silent overwrite).
	status, _, conflict := doV1(t, engine, gapsRedeemCall(secondWS, "BENE-OTHER", "beneficiary-key-1", 11))
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeRewardRedeemConflict) {
		t.Fatalf("hash conflict = %d %v, want 409 reward_redeem_conflict", status, conflict)
	}
}

func TestV1RewardBeneficiaryIgnoresOrganizationWorkspaceFunds(t *testing.T) {
	fx, rewards := beneficiaryFixture(t)
	orgWS := gapsWorkspace(t, fx)
	engine := fx.engine()

	status, _, payload := doV1(t, engine, gapsRedeemCall(orgWS, "BENE-2026", "beneficiary-org-key", 11))
	if status != http.StatusCreated {
		t.Fatalf("org-context redeem status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	// The edge names the caller's personal beneficiary, never the
	// organization workspace account; the service saw the caller, not an
	// organization payer.
	if beneficiary := data["billing_account_id"].(string); !strings.HasPrefix(beneficiary, "ba_beneficiary_") ||
		beneficiary == "ba_test_"+orgWS {
		t.Fatalf("beneficiary = %v, want caller personal ba_, never the workspace account", data)
	}
	if rewards.lastUserID != 11 {
		t.Fatalf("redeem user = %d, want caller 11", rewards.lastUserID)
	}
	if got := rewards.results[11].BillingAccountID; got != 8111 {
		t.Fatalf("service beneficiary account = %d, want caller personal 8111", got)
	}
}

func TestV1RewardBeneficiaryIdentityFailures(t *testing.T) {
	fx, _ := beneficiaryFixture(t)
	wsID := gapsWorkspace(t, fx)
	engine := fx.engine()

	status, _, hidden := doV1(t, engine, gapsRedeemCall(wsID, "BENE-2026", "beneficiary-id-key", 99))
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider redeem = %d %v, want 404 workspace_not_found", status, hidden)
	}
	status, _, anon := doV1(t, engine, gapsRedeemCall(wsID, "BENE-2026", "beneficiary-id-key", 0))
	if status != http.StatusUnauthorized {
		t.Fatalf("anon redeem = %d %v, want 401", status, anon)
	}
}

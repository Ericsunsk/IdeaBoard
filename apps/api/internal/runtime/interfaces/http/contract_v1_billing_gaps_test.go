package httpapi

// Workspace billing/payments-query/rewards-query gaps tests (issue
// #809-close, backend batch B).
//
// Fresh-DB friendly: in-memory tenancy store + fakes only, no testdb.
// Per-operation coverage incl. idempotent replay (same key+hash replays,
// same key+different hash 409s) and 404-vs-403 (outsiders converge to
// 404 workspace_not_found; anon is 401). Reuses the shared
// doV1/v1Data/v1ErrorCode helpers.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

const (
	gapOrderTokenA = "0198cd29-f457-7e4b-b95e-6cfde41b7222"
	gapOrderTokenB = "0198cd29-f457-7e4b-b95e-6cfde41b7333"
)

type fakeGapPurchaseOptions struct {
	mu      sync.Mutex
	options paymentcontracts.PurchaseOptions
	err     error
	calls   int
	lastUID int64
}

func (f *fakeGapPurchaseOptions) GetPurchaseOptions(ctx context.Context, userID int64) (paymentcontracts.PurchaseOptions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastUID = userID
	if f.err != nil {
		return paymentcontracts.PurchaseOptions{}, f.err
	}
	return f.options, nil
}

func gapDefaultPurchaseOptions() paymentcontracts.PurchaseOptions {
	return paymentcontracts.PurchaseOptions{
		PurchasesAvailable:  true,
		QuickIdeaQuantities: append([]int64(nil), paymentsdomain.QuickPurchaseIdeaQuantities[:]...),
		MinIdeaQuantity:     paymentsdomain.MinimumPurchaseIdeas,
		MaxIdeaQuantity:     paymentsdomain.MaximumPurchaseIdeas,
		IdeasPerUSD:         paymentsdomain.IdeasPerUSD,
		Terms:               paymentcontracts.PaymentLegalDocument{Version: paymentsdomain.CurrentTermsVersion, URL: "/terms"},
		RefundPolicy:        paymentcontracts.PaymentLegalDocument{Version: paymentsdomain.CurrentRefundPolicyVersion, URL: "/refund-policy"},
	}
}

func gapPendingOrder(token string, quantity int64, status string) paymentcontracts.PendingPaymentOrder {
	url := ""
	if status == string(paymentsdomain.PaymentOrderCheckoutOpen) {
		url = "https://checkout.example/s/" + token
	}
	return paymentcontracts.PendingPaymentOrder{
		PublicToken: token, IdeaQuantity: quantity, CheckoutURL: url, Status: status,
		ExpiresAt: billingTestNow.Add(time.Hour), UpdatedAt: billingTestNow,
	}
}

type fakeGapPendingOrders struct {
	mu           sync.Mutex
	orders       map[string]paymentcontracts.PendingPaymentOrder
	continueErr  error
	abandonErr   error
	continueKeys int
	abandonKeys  int
}

func (f *fakeGapPendingOrders) ListPendingOrders(ctx context.Context, userID int64) (paymentcontracts.PendingPaymentOrderList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := paymentcontracts.PendingPaymentOrderList{Items: []paymentcontracts.PendingPaymentOrder{}}
	for _, order := range f.orders {
		list.Items = append(list.Items, order)
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].PublicToken < list.Items[j].PublicToken })
	return list, nil
}

func (f *fakeGapPendingOrders) ContinuePendingOrder(ctx context.Context, userID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.continueErr != nil {
		return paymentcontracts.PendingPaymentOrder{}, f.continueErr
	}
	order, ok := f.orders[publicToken]
	if !ok {
		return paymentcontracts.PendingPaymentOrder{}, paymentcontracts.ErrPaymentOrderNotFound
	}
	f.continueKeys++
	return order, nil
}

func (f *fakeGapPendingOrders) AbandonPendingOrder(ctx context.Context, userID int64, publicToken string) (paymentcontracts.PendingPaymentOrder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.abandonErr != nil {
		return paymentcontracts.PendingPaymentOrder{}, f.abandonErr
	}
	order, ok := f.orders[publicToken]
	if !ok {
		return paymentcontracts.PendingPaymentOrder{}, paymentcontracts.ErrPaymentOrderNotFound
	}
	f.abandonKeys++
	order.Status = string(paymentsdomain.PaymentOrderCancelled)
	order.CheckoutURL = ""
	f.orders[publicToken] = order
	return order, nil
}

type fakeGapRecharges struct {
	mu    sync.Mutex
	items []paymentcontracts.RechargeHistoryItem
	err   error
}

func (f *fakeGapRecharges) ListRecharges(ctx context.Context, userID int64, cursor string, limit int) (paymentcontracts.RechargeHistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return paymentcontracts.RechargeHistoryPage{}, f.err
	}
	if limit <= 0 {
		return paymentcontracts.RechargeHistoryPage{}, paymentcontracts.ErrRechargeHistoryLimit
	}
	start := 0
	switch cursor {
	case "":
		start = 0
	case "gap-cursor-1":
		start = 2
	default:
		return paymentcontracts.RechargeHistoryPage{}, paymentcontracts.ErrRechargeHistoryCursor
	}
	if start > len(f.items) {
		return paymentcontracts.RechargeHistoryPage{}, paymentcontracts.ErrRechargeHistoryCursor
	}
	end := start + limit
	page := paymentcontracts.RechargeHistoryPage{Items: []paymentcontracts.RechargeHistoryItem{}}
	if end < len(f.items) {
		page.Items = append([]paymentcontracts.RechargeHistoryItem(nil), f.items[start:end]...)
		page.NextCursor = "gap-cursor-1"
		return page, nil
	}
	page.Items = append([]paymentcontracts.RechargeHistoryItem(nil), f.items[start:]...)
	return page, nil
}

func gapRechargeItem(quantity int64, status string) paymentcontracts.RechargeHistoryItem {
	return paymentcontracts.RechargeHistoryItem{
		OccurredAt: billingTestNow, IdeaQuantity: quantity,
		PaidAmountMinor: quantity, PaidCurrency: "usd", Status: status,
		ReceiptURL: "https://pay.stripe.com/receipts/" + status,
	}
}

type fakeGapQuoter struct {
	mu      sync.Mutex
	errFor  map[string]error
	unit    int64
	calls   int
	lastSKU string
}

func (f *fakeGapQuoter) QuoteAtResolution(ctx context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastSKU = rawSKU
	if err, ok := f.errFor[rawSKU]; ok {
		return billing.Quote{}, err
	}
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	unit := f.unit
	if unit <= 0 {
		unit = 3
	}
	if quantity <= 0 {
		return billing.Quote{}, billing.ErrInvalidQuantity
	}
	if unit != 0 && quantity > billing.MaxQuoteCredits/unit {
		return billing.Quote{}, billing.ErrQuoteTotalExceeded
	}
	return billing.Quote{
		PriceSnapshot: billing.PriceSnapshot{
			SKU: sku, DisplayName: "Test vision", PriceVersionID: 7, PriceVersion: 3,
			OutputResolution: resolution, UnitCredits: unit, QuotedAt: billingTestNow,
		},
		Quantity: quantity, TotalCredits: unit * quantity,
	}, nil
}

type fakeGapRewards struct {
	mu       sync.Mutex
	seq      int64
	redeemed map[string]ports.RedeemResult
	history  []ports.RedemptionHistoryItem
}

func (f *fakeGapRewards) Redeem(ctx context.Context, cmd ports.RedeemCommand) (ports.RedeemResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	code := strings.TrimSpace(cmd.Code)
	switch code {
	case "EXPIRED-CODE":
		return ports.RedeemResult{}, rewardsdomain.ErrCodeUnavailable
	case "FROZEN-CODE":
		return ports.RedeemResult{}, rewardsdomain.ErrWalletFrozen
	}
	if existing, ok := f.redeemed[code]; ok {
		replayed := existing
		replayed.Replayed = true
		return replayed, nil
	}
	f.seq++
	redemption := ports.Redemption{
		ID: f.seq, RewardCodeID: 100 + f.seq, UserID: cmd.UserID,
		CreditAmount: 80, CreditTransactionID: fmt.Sprintf("tx-reward-%d", f.seq),
		AvailableBalanceAfter: 1000 + 80*f.seq, RedeemedAt: billingTestNow,
		RewardCodeDisplayName: "Test reward", MaskedCodeHint: "IDEA••••1234",
	}
	result := ports.RedeemResult{
		Redemption:       redemption,
		HistoryItem:      ports.RedemptionHistoryItem{ID: redemption.ID, RewardCodeDisplayName: redemption.RewardCodeDisplayName, CreditedAmount: redemption.CreditAmount, MaskedCodeHint: redemption.MaskedCodeHint, RedeemedAt: redemption.RedeemedAt},
		CreditedAmount:   redemption.CreditAmount,
		AvailableBalance: redemption.AvailableBalanceAfter,
	}
	if f.redeemed == nil {
		f.redeemed = make(map[string]ports.RedeemResult)
	}
	f.redeemed[code] = result
	f.history = append([]ports.RedemptionHistoryItem{result.HistoryItem}, f.history...)
	return result, nil
}

func (f *fakeGapRewards) ListRedemptions(ctx context.Context, input ports.ListRedemptionsInput) (ports.RedemptionHistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	var filtered []ports.RedemptionHistoryItem
	for _, item := range f.history {
		if input.BeforeID <= 0 || item.ID < input.BeforeID {
			filtered = append(filtered, item)
		}
	}
	page := ports.RedemptionHistoryPage{}
	if len(filtered) > limit {
		page.Redemptions = append([]ports.RedemptionHistoryItem(nil), filtered[:limit]...)
		page.NextBeforeID = page.Redemptions[len(page.Redemptions)-1].ID
	} else {
		page.Redemptions = append([]ports.RedemptionHistoryItem(nil), filtered...)
	}
	return page, nil
}

type gapsTestFixture struct {
	store    *tenancy.InMemoryStore
	dir      *staticUserDirectory
	purchase *fakeGapPurchaseOptions
	pending  *fakeGapPendingOrders
	recharge *fakeGapRecharges
	quoter   *fakeGapQuoter
	rewards  *fakeGapRewards
	deps     ContractV1BillingDeps
}

func newGapsTestFixture() *gapsTestFixture {
	fx := &gapsTestFixture{
		store:    tenancy.NewInMemoryStoreForTests(),
		dir:      newStaticUserDirectory(),
		purchase: &fakeGapPurchaseOptions{options: gapDefaultPurchaseOptions()},
		pending:  &fakeGapPendingOrders{orders: map[string]paymentcontracts.PendingPaymentOrder{}},
		recharge: &fakeGapRecharges{},
		quoter:   &fakeGapQuoter{},
		rewards:  &fakeGapRewards{},
	}
	fx.deps = ContractV1BillingDeps{
		Tenancy: fx.store, Users: fx.dir,
		PurchaseOptions: fx.purchase, PendingOrders: fx.pending,
		Recharges: fx.recharge, Quoter: fx.quoter, Rewards: fx.rewards,
	}
	return fx
}

func (fx *gapsTestFixture) engine() *gin.Engine {
	engine := newV1TestEngine(fx.store, fx.dir)
	RegisterContractV1BillingRoutesOnEngine(engine, AuthDeps{}, fx.deps)
	return engine
}

func gapsWorkspace(t *testing.T, fx *gapsTestFixture) string {
	t.Helper()
	ws := v1WorkspaceFixture(t, context.Background(), fx.store, "gaps", 11)
	return ws.PublicID
}

func TestV1GapsPurchaseOptions(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	engine := fx.engine()

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/purchase-options", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("purchase-options status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["purchases_available"] != true || data["min_idea_quantity"] != float64(500) || data["ideas_per_usd"] != float64(100) {
		t.Fatalf("purchase options = %v, want server-side quote facts", data)
	}
	if fx.purchase.lastUID != 11 {
		t.Fatalf("purchase options user = %d, want caller 11 (not owner projection)", fx.purchase.lastUID)
	}

	// Member (non-owner) reads with their own facts; outsiders 404.
	if _, err := fx.store.AddWorkspaceMember(context.Background(), wsID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	status, _, member := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/purchase-options", userID: 12})
	if status != http.StatusOK {
		t.Fatalf("member purchase-options status = %d (%v)", status, member)
	}
	if fx.purchase.lastUID != 12 {
		t.Fatalf("member purchase options user = %d, want caller 12", fx.purchase.lastUID)
	}
	status, _, hidden := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/purchase-options", userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, hidden)
	}
	status, _, anon := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/purchase-options"})
	if status != http.StatusUnauthorized {
		t.Fatalf("anon = %d %v, want 401", status, anon)
	}
}

func TestV1GapsPendingOrdersList(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	fx.pending.orders[gapOrderTokenA] = gapPendingOrder(gapOrderTokenA, 1000, string(paymentsdomain.PaymentOrderCheckoutOpen))
	fx.pending.orders[gapOrderTokenB] = gapPendingOrder(gapOrderTokenB, 2000, string(paymentsdomain.PaymentOrderAsyncPaymentPending))
	engine := fx.engine()

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/pending-orders", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("pending-orders status = %d (%v)", status, payload)
	}
	items := v1Data(t, payload).(map[string]any)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("pending items = %v, want 2", payload)
	}
	first := items[0].(map[string]any)
	if first["public_token"] != gapOrderTokenA || first["checkout_url"] == "" {
		t.Fatalf("open order must carry checkout_url: %v", first)
	}
	second := items[1].(map[string]any)
	if _, hasURL := second["checkout_url"]; hasURL {
		t.Fatalf("non-open order must omit checkout_url: %v", second)
	}

	status, _, hidden := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/pending-orders", userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, hidden)
	}
}

func gapsContinueCall(path, orderID, key, body string, userID int64) v1Call {
	headers := map[string]string{}
	if key != "" {
		headers[contract.IdempotencyKeyHeader] = key
	}
	return v1Call{method: http.MethodPost, path: path + "/" + orderID + "/continue", body: body, userID: userID, headers: headers}
}

func TestV1GapsPendingContinue(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	fx.pending.orders[gapOrderTokenA] = gapPendingOrder(gapOrderTokenA, 1000, string(paymentsdomain.PaymentOrderCheckoutOpen))
	fx.pending.orders[gapOrderTokenB] = gapPendingOrder(gapOrderTokenB, 2000, string(paymentsdomain.PaymentOrderCheckoutOpen))
	engine := fx.engine()
	base := "/api/v1/workspaces/" + wsID + "/pending-orders"

	status, _, first := doV1(t, engine, gapsContinueCall(base, gapOrderTokenA, "continue-key-1", "", 11))
	if status != http.StatusOK {
		t.Fatalf("continue status = %d (%v)", status, first)
	}
	if v1Data(t, first).(map[string]any)["public_token"] != gapOrderTokenA {
		t.Fatalf("continue = %v, want order A", first)
	}

	// Same key + same order replays without a second service call.
	before := fx.pending.continueKeys
	status, _, replayed := doV1(t, engine, gapsContinueCall(base, gapOrderTokenA, "continue-key-1", "", 11))
	if status != http.StatusOK {
		t.Fatalf("continue replay status = %d (%v)", status, replayed)
	}
	if v1Data(t, replayed).(map[string]any)["public_token"] != gapOrderTokenA {
		t.Fatalf("replay = %v, want order A", replayed)
	}
	if fx.pending.continueKeys != before {
		t.Fatal("same key + same hash must replay, not re-execute")
	}

	// Same key + different order conflicts.
	status, _, conflict := doV1(t, engine, gapsContinueCall(base, gapOrderTokenB, "continue-key-1", "", 11))
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(CodeBillingCheckoutConflict) {
		t.Fatalf("hash conflict = %d %v, want 409 billing_checkout_conflict", status, conflict)
	}

	// Missing key, unknown token, malformed token, non-empty body.
	status, _, missing := doV1(t, engine, gapsContinueCall(base, gapOrderTokenA, "", "", 11))
	if status != http.StatusBadRequest || v1ErrorCode(t, missing) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, missing)
	}
	status, _, unknown := doV1(t, engine, gapsContinueCall(base, "0198cd29-f457-7e4b-b95e-6cfde41b7999", "continue-key-2", "", 11))
	if status != http.StatusNotFound || v1ErrorCode(t, unknown) != string(CodeBillingSessionNotFound) {
		t.Fatalf("unknown token = %d %v, want 404 billing_session_not_found", status, unknown)
	}
	status, _, malformed := doV1(t, engine, gapsContinueCall(base, "not-a-uuid", "continue-key-3", "", 11))
	if status != http.StatusNotFound || v1ErrorCode(t, malformed) != string(CodeBillingSessionNotFound) {
		t.Fatalf("malformed token = %d %v, want 404 billing_session_not_found (no oracle)", status, malformed)
	}
	status, _, bodied := doV1(t, engine, gapsContinueCall(base, gapOrderTokenA, "continue-key-4", `{"data":{}}`, 11))
	if status != http.StatusBadRequest || v1ErrorCode(t, bodied) != string(contract.CodeBadRequest) {
		t.Fatalf("non-empty body = %d %v, want 400 bad_request", status, bodied)
	}

	// Wallet gate maps to the frozen billing code.
	fx.pending.continueErr = paymentsdomain.ErrWalletNotRechargeable
	status, _, gated := doV1(t, engine, gapsContinueCall(base, gapOrderTokenA, "continue-key-5", "", 11))
	if status != http.StatusForbidden || v1ErrorCode(t, gated) != string(CodeBillingWalletRestricted) {
		t.Fatalf("wallet gate = %d %v, want 403 billing_wallet_restricted", status, gated)
	}
	fx.pending.continueErr = nil

	// Cross-workspace invisibility first (404 before order lookup).
	status, _, hidden := doV1(t, engine, gapsContinueCall("/api/v1/workspaces/"+wsID+"/pending-orders", gapOrderTokenA, "continue-key-6", "", 99))
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider continue = %d %v, want 404 workspace_not_found", status, hidden)
	}
}

func TestV1GapsPendingAbandon(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	fx.pending.orders[gapOrderTokenA] = gapPendingOrder(gapOrderTokenA, 1000, string(paymentsdomain.PaymentOrderCheckoutOpen))
	engine := fx.engine()
	call := func(orderID, key string) (int, map[string]any) {
		headers := map[string]string{}
		if key != "" {
			headers[contract.IdempotencyKeyHeader] = key
		}
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: "/api/v1/workspaces/" + wsID + "/pending-orders/" + orderID + "/abandon",
			userID: 11, headers: headers,
		})
		return status, payload
	}

	status, first := call(gapOrderTokenA, "abandon-key-1")
	if status != http.StatusOK {
		t.Fatalf("abandon status = %d (%v)", status, first)
	}
	if got := v1Data(t, first).(map[string]any)["status"]; got != string(paymentsdomain.PaymentOrderCancelled) {
		t.Fatalf("abandon status = %v, want cancelled", first)
	}
	status, replayed := call(gapOrderTokenA, "abandon-key-1")
	if status != http.StatusOK || v1Data(t, replayed).(map[string]any)["status"] != string(paymentsdomain.PaymentOrderCancelled) {
		t.Fatalf("abandon replay = %d %v, want replayed cancelled", status, replayed)
	}
	status, unknown := call("0198cd29-f457-7e4b-b95e-6cfde41b7999", "abandon-key-2")
	if status != http.StatusNotFound || v1ErrorCode(t, unknown) != string(CodeBillingSessionNotFound) {
		t.Fatalf("unknown abandon = %d %v, want 404 billing_session_not_found", status, unknown)
	}
}

func TestV1GapsRecharges(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	fx.recharge.items = []paymentcontracts.RechargeHistoryItem{
		gapRechargeItem(1000, string(paymentsdomain.PaymentOrderCredited)),
		gapRechargeItem(2000, "refunded"),
		gapRechargeItem(500, string(paymentsdomain.PaymentOrderCancelled)),
	}
	engine := fx.engine()
	base := "/api/v1/workspaces/" + wsID + "/recharges"

	status, _, first := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?page_size=2", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("recharges p1 status = %d (%v)", status, first)
	}
	if len(first["data"].([]any)) != 2 {
		t.Fatalf("recharges p1 = %v, want 2 items", first)
	}
	meta, _ := first["meta"].(map[string]any)
	if meta["has_more"] != true {
		t.Fatalf("recharges p1 meta = %v, want has_more=true", first["meta"])
	}
	next, _ := meta["next_cursor"].(string)
	if next == "" {
		t.Fatalf("recharges p1 must carry next_cursor: %v", first["meta"])
	}
	entry := first["data"].([]any)[0].(map[string]any)
	if entry["receipt_url"] == "" || entry["occurred_at"] == "" || entry["idea_quantity"] != float64(1000) {
		t.Fatalf("recharge entry = %v, want receipt + occurred_at + quantity", entry)
	}

	status, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?cursor=" + next, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("recharges p2 status = %d (%v)", status, second)
	}
	if len(second["data"].([]any)) != 1 {
		t.Fatalf("recharges p2 = %v, want 1 item", second)
	}
	if second["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("recharges p2 meta = %v, want exhausted", second["meta"])
	}

	status, _, bad := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?cursor=bogus", userID: 11})
	if status != http.StatusBadRequest || v1ErrorCode(t, bad) != string(contract.CodePaginationCursorInvalid) {
		t.Fatalf("bad cursor = %d %v, want 400 pagination_cursor_invalid", status, bad)
	}
	status, _, sized := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?page_size=0", userID: 11})
	if status != http.StatusBadRequest || v1ErrorCode(t, sized) != string(contract.CodeBadRequest) {
		t.Fatalf("bad page_size = %d %v, want 400 bad_request", status, sized)
	}
	status, _, hidden := doV1(t, engine, v1Call{method: http.MethodGet, path: base, userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, hidden)
	}
}

func TestV1GapsBillingQuote(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	fx.quoter.errFor = map[string]error{"zzz:unknown-thing": billing.ErrSKUNotFound}
	engine := fx.engine()
	base := "/api/v1/workspaces/" + wsID + "/billing-quotes"

	// Vision price preflight (the visionBillingResource path): defaults
	// resolve to output_resolution=default, quantity=1.
	status, _, quoted := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base,
		body: `{"data":{"sku":"ai-search:vision"}}`, userID: 11,
	})
	if status != http.StatusOK {
		t.Fatalf("vision quote status = %d (%v)", status, quoted)
	}
	vision := v1Data(t, quoted).(map[string]any)
	if vision["sku"] != "ai-search:vision" || vision["total_credits"] != float64(3) || vision["quantity"] != float64(1) {
		t.Fatalf("vision quote = %v, want immutable snapshot total 3", vision)
	}
	if vision["output_resolution"] != "default" || vision["quoted_at"] == "" {
		t.Fatalf("vision quote must pin resolution + quoted_at: %v", vision)
	}
	if fx.quoter.lastSKU != "ai-search:vision" {
		t.Fatalf("quoter sku = %q, want canonical ai-search:vision", fx.quoter.lastSKU)
	}

	status, _, sized := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base,
		body: `{"data":{"sku":"ai-search:vision","quantity":2}}`, userID: 11,
	})
	if status != http.StatusOK || v1Data(t, sized).(map[string]any)["total_credits"] != float64(6) {
		t.Fatalf("quantity quote = %d %v, want total 6", status, sized)
	}

	// Unpriced/unregistered SKUs are deterministic 422s (no retry, no
	// oracle), whether the catalog surfaces ErrPricingUnavailable or
	// ErrSKUNotFound; client price input is strictly rejected; bad
	// quantities are 422.
	fx.quoter.errFor["zzz:unpriced-thing"] = billing.ErrPricingUnavailable
	for _, sku := range []string{"zzz:unknown-thing", "zzz:unpriced-thing"} {
		status, _, priced := doV1(t, engine, v1Call{
			method: http.MethodPost, path: base,
			body: fmt.Sprintf(`{"data":{"sku":%q}}`, sku), userID: 11,
		})
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, priced) != string(CodeBillingPricingUnavailable) {
			t.Fatalf("unpriced sku %s = %d %v, want 422 billing_pricing_unavailable", sku, status, priced)
		}
	}

	// Transient quoter failures stay 503 (retryable).
	fx.quoter.errFor["ai-search:vision"] = billing.ErrServiceUnavailable
	status, _, transient := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base,
		body: `{"data":{"sku":"ai-search:vision"}}`, userID: 11,
	})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, transient) != string(CodeBillingQuoteUnavailable) {
		t.Fatalf("transient quote = %d %v, want 503 billing_quote_unavailable", status, transient)
	}
	delete(fx.quoter.errFor, "ai-search:vision")
	status, _, priced := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base,
		body: `{"data":{"sku":"ai-search:vision","unit_price":5}}`, userID: 11,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("client price input = %d (%v), want 400 strict rejection", status, priced)
	}
	status, _, zero := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base,
		body: `{"data":{"sku":"ai-search:vision","quantity":0}}`, userID: 11,
	})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, zero) != string(contract.CodeValidationFailed) {
		t.Fatalf("zero quantity = %d %v, want 422 validation_failed", status, zero)
	}
	status, _, hidden := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base,
		body: `{"data":{"sku":"ai-search:vision"}}`, userID: 99,
	})
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider quote = %d %v, want 404 workspace_not_found", status, hidden)
	}
}

func gapsRedeemCall(wsID, code, key string, userID int64) v1Call {
	headers := map[string]string{}
	if key != "" {
		headers[contract.IdempotencyKeyHeader] = key
	}
	return v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + wsID + "/rewards/redemptions",
		body: fmt.Sprintf(`{"data":{"code":%q}}`, code), userID: userID, headers: headers,
	}
}

func TestV1GapsRewardRedeem(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	engine := fx.engine()
	wantUser := testUUIDForUser(11).String()

	status, _, first := doV1(t, engine, gapsRedeemCall(wsID, "IDEA-2026", "redeem-key-1", 11))
	if status != http.StatusCreated {
		t.Fatalf("redeem status = %d (%v)", status, first)
	}
	data := v1Data(t, first).(map[string]any)
	redemption := data["redemption"].(map[string]any)
	if redemption["credit_amount"] != float64(80) || redemption["user_public_id"] != wantUser || redemption["display_name"] != "test-user-11" {
		t.Fatalf("redemption = %v, want credited caller snapshot", redemption)
	}
	if data["credited_amount"] != float64(80) || data["available_balance"] != float64(1080) {
		t.Fatalf("redeem result = %v, want amounts", data)
	}
	if replayed, _ := data["replayed"].(bool); replayed {
		t.Fatalf("first redeem must not be marked replayed: %v", data)
	}
	firstID := redemption["id"]

	// Same key + same code replays the first redemption.
	status, _, replayed := doV1(t, engine, gapsRedeemCall(wsID, "IDEA-2026", "redeem-key-1", 11))
	if status != http.StatusCreated {
		t.Fatalf("redeem replay status = %d (%v)", status, replayed)
	}
	replayData := v1Data(t, replayed).(map[string]any)
	if replayData["redemption"].(map[string]any)["id"] != firstID {
		t.Fatalf("replay = %v, want same redemption id %v", replayed, firstID)
	}
	if replayData["replayed"] != true {
		t.Fatalf("replay must carry replayed=true: %v", replayed)
	}

	// Same key + different code conflicts.
	status, _, conflict := doV1(t, engine, gapsRedeemCall(wsID, "IDEA-2027", "redeem-key-1", 11))
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(CodeRewardRedeemConflict) {
		t.Fatalf("hash conflict = %d %v, want 409 reward_redeem_conflict", status, conflict)
	}

	// Missing key, invalid code, empty code.
	status, _, missing := doV1(t, engine, gapsRedeemCall(wsID, "IDEA-2026", "", 11))
	if status != http.StatusBadRequest || v1ErrorCode(t, missing) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, missing)
	}
	status, _, invalid := doV1(t, engine, gapsRedeemCall(wsID, "EXPIRED-CODE", "redeem-key-2", 11))
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, invalid) != string(CodeRewardCodeUnavailable) {
		t.Fatalf("invalid code = %d %v, want 422 reward_code_unavailable", status, invalid)
	}
	status, _, empty := doV1(t, engine, gapsRedeemCall(wsID, "   ", "redeem-key-3", 11))
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, empty) != string(contract.CodeValidationFailed) {
		t.Fatalf("empty code = %d %v, want 422 validation_failed", status, empty)
	}
	status, _, frozen := doV1(t, engine, gapsRedeemCall(wsID, "FROZEN-CODE", "redeem-key-4", 11))
	if status != http.StatusConflict || v1ErrorCode(t, frozen) != string(CodeRewardWalletLocked) {
		t.Fatalf("frozen wallet = %d %v, want 409 reward_wallet_locked", status, frozen)
	}

	// Cross-workspace invisibility first (404 before redeem).
	status, _, hidden := doV1(t, engine, gapsRedeemCall(wsID, "IDEA-2026", "redeem-key-5", 99))
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider redeem = %d %v, want 404 workspace_not_found", status, hidden)
	}
}

func TestV1GapsRewardRedemptionHistory(t *testing.T) {
	fx := newGapsTestFixture()
	wsID := gapsWorkspace(t, fx)
	engine := fx.engine()
	for i, code := range []string{"IDEA-A", "IDEA-B"} {
		status, _, payload := doV1(t, engine, gapsRedeemCall(wsID, code, fmt.Sprintf("hist-key-%d", i), 11))
		if status != http.StatusCreated {
			t.Fatalf("seed redeem %s = %d (%v)", code, status, payload)
		}
	}
	base := "/api/v1/workspaces/" + wsID + "/rewards/redemptions"

	status, _, first := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?page_size=1", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("history p1 status = %d (%v)", status, first)
	}
	if len(first["data"].([]any)) != 1 {
		t.Fatalf("history p1 = %v, want 1 item", first)
	}
	meta, _ := first["meta"].(map[string]any)
	if meta["has_more"] != true {
		t.Fatalf("history p1 meta = %v, want has_more=true", first["meta"])
	}
	next, _ := meta["next_cursor"].(string)
	if next == "" {
		t.Fatalf("history p1 must carry next_cursor: %v", first["meta"])
	}
	item := first["data"].([]any)[0].(map[string]any)
	if item["reward_code_display_name"] != "Test reward" || item["credited_amount"] != float64(80) || item["redeemed_at"] == "" {
		t.Fatalf("history item = %v, want display + amount + time", item)
	}

	status, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?cursor=" + next, userID: 11})
	if status != http.StatusOK || len(second["data"].([]any)) != 1 {
		t.Fatalf("history p2 = %d %v, want 1 item", status, second)
	}
	if second["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("history p2 meta = %v, want exhausted", second["meta"])
	}

	status, _, bad := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "?cursor=!!!", userID: 11})
	if status != http.StatusBadRequest || v1ErrorCode(t, bad) != string(contract.CodePaginationCursorInvalid) {
		t.Fatalf("bad cursor = %d %v, want 400 pagination_cursor_invalid", status, bad)
	}
	status, _, hidden := doV1(t, engine, v1Call{method: http.MethodGet, path: base, userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, hidden) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider history = %d %v, want 404 workspace_not_found", status, hidden)
	}
}

// TestV1GapsFailClosedWithoutSeams keeps every gaps route fail-closed when
// production has not wired the seams (zero Deps): reads are 503, never 500
// with an empty envelope and never a ledger write.
func TestV1GapsFailClosedWithoutSeams(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	ws := v1WorkspaceFixture(t, context.Background(), store, "gaps", 11)
	engine := newV1TestEngine(store, dir)
	RegisterContractV1BillingRoutesOnEngine(engine, AuthDeps{}, ContractV1BillingDeps{Tenancy: store, Users: dir})

	gets := []string{
		"/api/v1/workspaces/" + ws.PublicID + "/purchase-options",
		"/api/v1/workspaces/" + ws.PublicID + "/pending-orders",
		"/api/v1/workspaces/" + ws.PublicID + "/recharges",
		"/api/v1/workspaces/" + ws.PublicID + "/rewards/redemptions",
	}
	for _, path := range gets {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
		if status != http.StatusServiceUnavailable {
			t.Fatalf("GET %s unwired = %d %v, want 503", path, status, payload)
		}
	}
	status, _, quoted := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-quotes",
		body: `{"data":{"sku":"ai-search:vision"}}`, userID: 11,
	})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, quoted) != string(CodeBillingQuoteUnavailable) {
		t.Fatalf("unwired quote = %d %v, want 503 billing_quote_unavailable", status, quoted)
	}
	posts := []v1Call{
		{method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/pending-orders/" + gapOrderTokenA + "/continue", userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "k1"}},
		{method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/pending-orders/" + gapOrderTokenA + "/abandon", userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "k2"}},
		{method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/rewards/redemptions", body: `{"data":{"code":"X"}}`, userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "k3"}},
	}
	for _, call := range posts {
		status, _, payload := doV1(t, engine, call)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("POST %s unwired = %d %v, want 503", call.path, status, payload)
		}
	}
}

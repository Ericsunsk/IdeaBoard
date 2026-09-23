package tests

import (
	payments "ideaboard/api/internal/payments"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"net/http"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/identity/principal"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// createCreditedPaginationOrder persists one credited order with a distinct
// keyset position (paid_at, id) so pagination tests can walk a stable chain.
func createCreditedPaginationOrder(t *testing.T, repo *paymentpostgres.Repository, credentialID string, userID int64, seq int, paidAt time.Time) {
	t.Helper()
	seed := fmt.Sprintf("pagination-order-%d", seq)
	hash := sha256.Sum256([]byte(seed))
	ideaQuantity := int64(500 + seq*100)
	input := paymentsdomain.CreatePaymentOrderIntentInput{
		ID: idempotentUUID(seed + "-id"), PublicOrderNumber: fmt.Sprintf("IBP-20260801-PAG%09d", seq),
		PublicToken: idempotentUUID(seed + "-token"), UserID: userID,
		ClientIdempotencyKey: seed, RequestHash: hex.EncodeToString(hash[:]),
		StripeCredentialID: credentialID, StripeAccountID: "acct_pagination", StripeProductID: "prod_pagination", IdeaQuantity: ideaQuantity,
		TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "http://localhost:5173", ExpiresAt: paidAt.Add(24 * time.Hour),
	}
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	order, err = repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_pagination", StripeSessionID: fmt.Sprintf("cs_pagination_%d", seq),
		CheckoutURL: fmt.Sprintf("https://checkout.stripe.com/c/pay/cs_pagination_%d", seq), ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	facts := paidCheckoutFacts(order)
	facts.PaidAt = paidAt
	facts.StripePaymentIntentID = fmt.Sprintf("pi_pagination_%d", seq)
	facts.StripeChargeID = fmt.Sprintf("ch_pagination_%d", seq)
	if _, err := repo.ConvergePaidPaymentOrder(t.Context(), facts, paidAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func idempotentUUID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	var id [16]byte
	copy(id[:], sum[:16])
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	parsed, err := uuid.FromBytes(id[:])
	if err != nil {
		panic(err)
	}
	return parsed.String()
}

func newPaginationTestService(t *testing.T, repo *paymentpostgres.Repository, wallets ports.WalletReader, now time.Time) *payments.CheckoutWorkflow {
	t.Helper()
	return newCompleteTestCheckoutWorkflow(t, repo, payments.NewPaymentRuntime(nil, func() time.Time { return now }), testStripeGateway{}, testCheckoutGateway{}, wallets, testShutdownEnqueuer{})
}

func TestListRechargesPaginatesAcrossFullHistory(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	repo := paymentpostgres.NewRepository(db)
	configuration := publishTestStripeConfiguration(t, repo, "acct_pagination", "prod_pagination")
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	for seq, hoursAgo := range []int{1, 2, 3} {
		createCreditedPaginationOrder(t, repo, configuration.CredentialID, 42, seq, now.Add(-time.Duration(hoursAgo)*time.Hour))
	}
	service := newPaginationTestService(t, repo, creditService, now)

	first, err := service.ListRecharges(t.Context(), 42, "", 2)
	if err != nil {
		t.Fatalf("ListRecharges() first page error = %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want 2 items with next cursor", first)
	}
	if first.Items[0].IdeaQuantity != 500 || first.Items[1].IdeaQuantity != 600 {
		t.Fatalf("first page order = %+v, want newest first", first.Items)
	}

	second, err := service.ListRecharges(t.Context(), 42, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("ListRecharges() second page error = %v", err)
	}
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want final item without next cursor", second)
	}
	if second.Items[0].IdeaQuantity != 700 {
		t.Fatalf("second page item = %+v, want oldest order", second.Items[0])
	}

	full, err := service.ListRecharges(t.Context(), 42, "", 0)
	if err != nil {
		t.Fatalf("ListRecharges() default limit error = %v", err)
	}
	if len(full.Items) != 3 || full.NextCursor != "" {
		t.Fatalf("default limit page = %+v, want all 3 items without next cursor", full)
	}
}

func TestListRechargesRejectsInvalidLimitAndCursorWithStableCodes(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	repo := paymentpostgres.NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_pagination_http", "prod_pagination_http")
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	service := newPaginationTestService(t, repo, credits.NewService(pool), now)
	handler := paymenthttp.NewUserCheckoutHandler(service, testPublicOrigins("https://configured.example.com"))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.GET("/user/payments/recharges", handler.ListRecharges)

	cases := []struct {
		name string
		path string
		code string
	}{
		{name: "zero limit", path: "/user/payments/recharges?limit=0", code: "recharge_history_limit_invalid"},
		{name: "over max limit", path: "/user/payments/recharges?limit=201", code: "recharge_history_limit_invalid"},
		{name: "non-numeric limit", path: "/user/payments/recharges?limit=x", code: "recharge_history_limit_invalid"},
		{name: "bad cursor", path: "/user/payments/recharges?cursor=bad", code: "recharge_history_cursor_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := performPaymentRequest(router, http.MethodGet, tc.path, "", nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("GET %s status = %d, want 400 (body %s)", tc.path, response.Code, response.Body.String())
			}
			var payload struct {
				Code        string `json:"code"`
				MessageSafe bool   `json:"message_safe"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != tc.code || !payload.MessageSafe {
				t.Fatalf("GET %s payload = %s, want code %q with message_safe=true", tc.path, response.Body.String(), tc.code)
			}
		})
	}

	valid := performPaymentRequest(router, http.MethodGet, "/user/payments/recharges?limit=50", "", nil)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid pagination status = %d, body %s", valid.Code, valid.Body.String())
	}
	if !strings.Contains(valid.Body.String(), `"items"`) {
		t.Fatalf("valid pagination body = %s, want items page", valid.Body.String())
	}
}

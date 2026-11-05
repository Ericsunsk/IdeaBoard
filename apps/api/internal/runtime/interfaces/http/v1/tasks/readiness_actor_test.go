package tasks

// Readiness/Quote actor consistency (#980, part of #973).
//
// Personal workspaces bill payer = actor: the quote/readiness preflight
// must resolve the SAME actor account submit holds (same funds identity,
// same wallet balance, same quota skip), with the Board generate gate
// unchanged. This file pins the actor order for the HTTP preflight with
// in-memory seams (no database); Fresh DB terminal convergence lives in
// workspacetasks/settle_readiness_convergence_test.go.

import (
	"context"
	"fmt"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	taskadapters "ideaboard/api/internal/tasking/infrastructure/adapters"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1testsupport "ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	"ideaboard/api/internal/tenancy/funds"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// v1ActorBillingAccounts models the Personal payer order: both the submit
// seam and the quote preflight name the caller's own account (8000+actor),
// never the workspace owner's. Shared=false so the Org-only quota gate
// skips without consulting the checker.
type v1ActorBillingAccounts struct{}

func actorAccountFor(userID int64) int64 { return 8000 + userID }

func (v1ActorBillingAccounts) ResolveBillingAccountInTx(_ context.Context, _ pgx.Tx, _ string, actorUserID int64) (int64, string, error) {
	return actorAccountFor(actorUserID), tenancypg.BillingOwnerWorkspace, nil
}

func (v1ActorBillingAccounts) ResolveFundsForActor(_ context.Context, workspacePublicID string, actorUserID int64) (funds.Identity, error) {
	account := actorAccountFor(actorUserID)
	return funds.Identity{
		WorkspaceInternalID:    7001,
		WorkspacePublicID:      workspacePublicID,
		BillingAccountID:       account,
		BillingAccountPublicID: fmt.Sprintf("ba_actor%07d", actorUserID),
		OwnerType:              tenancypg.BillingOwnerWorkspace,
	}, nil
}

// v1ActorQuoteWallets serves per-account balances so the preflight can
// prove it reads the actor's wallet, not the owner's.
type v1ActorQuoteWallets struct {
	balances map[int64]int64
}

func newV1ActorQuoteWallets() *v1ActorQuoteWallets {
	return &v1ActorQuoteWallets{balances: map[int64]int64{7001: 1000000}}
}

func (f *v1ActorQuoteWallets) fund(accountID, available int64) {
	if f.balances == nil {
		f.balances = map[int64]int64{}
	}
	f.balances[accountID] = available
}

func (f *v1ActorQuoteWallets) GetWallet(_ context.Context, accountID int64) (domain.WalletDetails, error) {
	available := int64(1000000)
	if v, ok := f.balances[accountID]; ok {
		available = v
	}
	details := domain.WalletDetails{Status: domain.WalletStatusActive}
	details.Available = available
	return details, nil
}

// newQuotaTaskEngineWithAccountsAndWallets mirrors
// newQuotaTaskEngineWithAccounts but with per-actor wallets so the
// preflight can prove it reads the actor's balance, not the owner's.
func newQuotaTaskEngineWithAccountsAndWallets(fx *v1TaskFixture, quotas *stubTaskQuotas, accounts v1ActorBillingAccounts, wallets *v1ActorQuoteWallets) *gin.Engine {
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
		if role == "" {
			role = "user"
		}
		id := fx.dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	handler := NewHandler(Deps{
		Store: fx.store, Users: fx.dir,
		Tasks: ContractV1TaskDeps{
			Tasks: fx.txTasks, TaskLifecycle: fx.txTasks, TaskSubmissionClaims: fx.txTasks,
			TaskQuoter: fx.quoter, TaskHolder: fx.holder, TaskQueue: fx.queue, TaskAssets: fx.assets,
			TaskRouter:          allowTestPoolResolver(),
			TaskSnapshotSaver:   v1testsupport.TxSnapshotSaver(fx.saved),
			TaskBillingAccounts: accounts,
			TaskQuotas:          quotas,
			TaskReadiness: taskapp.NewReadiness(taskapp.ReadinessDeps{
				Workspaces: &taskadapters.ReadinessWorkspaceReader{Store: fx.store},
				Boards:     nil, Quoter: fx.quoter, Resolver: allowTestPoolResolver(),
				BillingAccounts: accounts, Wallets: wallets, Quotas: quotas,
			}),
			TaskAcceptance: &v1testsupport.TxSubmissionAcceptance{Assets: fx.assets},
		},
	})
	handler.Mount(v1)
	return router
}

func TestV1TaskQuoteAndSubmitShareActorAccount(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	accounts := v1ActorBillingAccounts{}
	wallets := newV1ActorQuoteWallets()
	// Owner broke, actors funded: the preflight must read the actor, not
	// the owner. Before the #980 fix it reads 7001 (5) and reports 402;
	// after the fix it reads 8000+actor and reports 200, matching submit.
	wallets.fund(7001, 5)
	wallets.fund(actorAccountFor(2), 1000000)
	wallets.fund(actorAccountFor(3), 1000000)
	quotas := &stubTaskQuotas{}
	engine := newQuotaTaskEngineWithAccountsAndWallets(fx, quotas, accounts, wallets)

	// Quote as user 3 must name the actor account submit holds.
	status, quotePayload := quoteTask(t, engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("quote = %d %v, want 200 (actor wallet, not owner)", status, quotePayload)
	}
	// Submit as the same user must hold the same actor account.
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "980-actor-share-1"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("submit = %d %v, want 202", status, payload)
	}
	taskID := v1Data(t, payload).(map[string]any)["id"].(string)
	stored, err := fx.tasks.GetTask(context.Background(), fx.ws.PublicID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if want := actorAccountFor(3); stored.BillingAccountID != want {
		t.Fatalf("task billing account = %d, want actor %d (quote must share it)", stored.BillingAccountID, want)
	}
	if want := actorAccountFor(3); stored.BillingSnapshot.BillingAccountID != want {
		t.Fatalf("snapshot billing account = %d, want actor %d", stored.BillingSnapshot.BillingAccountID, want)
	}
	// A different member bills a different account: no cross pollution.
	status, _, payload = doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 2,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "980-actor-share-2"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("second submit = %d %v, want 202", status, payload)
	}
	secondID := v1Data(t, payload).(map[string]any)["id"].(string)
	second, err := fx.tasks.GetTask(context.Background(), fx.ws.PublicID, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if want := actorAccountFor(2); second.BillingAccountID != want {
		t.Fatalf("second task billing account = %d, want actor %d", second.BillingAccountID, want)
	}
	if second.BillingAccountID == stored.BillingAccountID {
		t.Fatal("two actors share one billing account, want per-actor accounts")
	}
}

package workspacetasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func canonicalQuantityParams(t *testing.T, feature string, count int) string {
	t.Helper()
	plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: feature,
		Params:  workflow.FeatureExecutionParams{RequestedOutputCount: &count, ImageInputCount: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := workflow.EncodeFeatureExecutionPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	params, err := json.Marshal(tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion, Service: feature, ExecutionPlan: encoded,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(params)
}

func TestCreateRejectsQuantityConflictBeforeSettlement(t *testing.T) {
	for _, quantity := range []int64{1, 5, -1} {
		t.Run(strconv.FormatInt(quantity, 10), func(t *testing.T) {
			store, holder, queue, service := planTestService()
			_, err := service.Create(t.Context(), CreateInput{
				WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Quantity: quantity,
				CanonicalParams: canonicalQuantityParams(t, "render", 4),
			}, "quantity-conflict")
			var validation *tasks.ValidationError
			if !errors.As(err, &validation) || validation.Fields["quantity"] == "" {
				t.Fatalf("Create error = %v, want quantity validation", err)
			}
			count, _ := store.CountTasks(t.Context(), "ws_abc")
			if count != 0 || len(holder.holds) != 0 || queue.calls != 0 || service.quoter.(*fakeQuoter).calls != 0 {
				t.Fatal("quantity conflict reached Quote/Hold/Task insert/enqueue")
			}
		})
	}
}

func TestAtomicQuantityBindsHoldAndActualOutputSettlement(t *testing.T) {
	env := openAtomicEnv(t, 10)
	input := CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1,
		CanonicalParams: canonicalQuantityParams(t, "render", 4),
	}
	if _, err := env.service.Create(t.Context(), input, "quantity-plan"); !IsValidationError(err) {
		t.Fatalf("conflicting Create = %v, want validation error", err)
	}
	for _, table := range []string{"tasks", "task_submission_idempotency", "task_billing_snapshots", "task_billing_settlements", "settle_enqueue_probe"} {
		var count int
		if err := env.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rejected quantity left %s count=%d, error=%v", table, count, err)
		}
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 || env.ledgerCount(t, `command_type = 'hold'`) != 0 {
		t.Fatal("conflicting quantity changed the wallet or created a Hold")
	}
	input.Quantity = 0
	created, err := env.service.Create(t.Context(), input, "quantity-plan")
	if err != nil {
		t.Fatal(err)
	}
	if created.BillingSnapshot.Quantity != 4 || env.probeCount(t, created.ID) != 1 {
		t.Fatalf("accepted Task billing=%+v, want quantity=4 and one enqueue", created.BillingSnapshot)
	}
	if available, reserved := env.wallet(t); available != 960 || reserved != 40 {
		t.Fatalf("wallet after Hold=(%d,%d), want (960,40)", available, reserved)
	}
	env.claimTask(t, created.ID, created.ExecutionToken)
	outputs := make([]OutputRef, 4)
	for i := range outputs {
		outputs[i] = OutputRef{Kind: OutputKindRemote, URL: fmt.Sprintf("https://cdn.example.com/quantity-%d.png", i), ChecksumSHA256: atomicTestChecksum, ResultIndex: i}
	}
	completed, err := env.service.Complete(t.Context(), env.ws, created.ID, outputs)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || len(completed.OutputRefs) != 4 || completed.BillingSnapshot.SettlementState != "captured" {
		t.Fatalf("four-output settlement = %+v", completed)
	}
	if available, reserved := env.wallet(t); available != 960 || reserved != 0 {
		t.Fatalf("wallet after Capture=(%d,%d), want (960,0)", available, reserved)
	}
}

func TestCreateDerivesQuantityFromExecutionPlan(t *testing.T) {
	for _, quantity := range []int64{0, 4} {
		_, _, queue, service := planTestService()
		service.quoter = &fakeQuoter{snapshot: BillingSnapshot{UnitCredits: 10}}
		created, err := service.Create(t.Context(), CreateInput{
			WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render", Quantity: quantity,
			CanonicalParams: canonicalQuantityParams(t, "render", 4),
		}, "quantity-derived")
		if err != nil {
			t.Fatal(err)
		}
		if created.BillingSnapshot.Quantity != 4 || created.BillingSnapshot.TotalCredits != 40 || len(queue.enqueued) != 1 {
			t.Fatalf("Task billing = %+v, want four quoted outputs and one enqueue", created.BillingSnapshot)
		}
	}
}

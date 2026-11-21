package tasks

import (
	"fmt"
	"net/http"
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestV1TaskQuantityMatchesExecutionPlan(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		service  string
		params   string
		quantity string
		want     int64
	}{
		{name: "default", service: "generation", params: `{}`, want: 1},
		{name: "derive two", service: "generation", params: `{"output_count":2}`, want: 2},
		{name: "derive four", service: "generation", params: `{"output_count":4}`, want: 4},
		{name: "explicit agreement", service: "generation", params: `{"output_count":3}`, quantity: `,"quantity":3`, want: 3},
		{name: "underquote", service: "generation", params: `{"output_count":4}`, quantity: `,"quantity":1`},
		{name: "overquote", service: "generation", params: `{"output_count":1}`, quantity: `,"quantity":4`},
		{name: "omitted output count", service: "generation", params: `{}`, quantity: `,"quantity":4`},
		{name: "zero", service: "generation", params: `{"output_count":4}`, quantity: `,"quantity":0`},
		{name: "negative", service: "generation", params: `{"output_count":4}`, quantity: `,"quantity":-1`},
		{name: "fixed cutout", service: "cutout", params: `{}`, want: 1},
		{name: "fixed conflict", service: "cutout", params: `{}`, quantity: `,"quantity":2`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fx := newV1PromptMaskFixture(t)
			assets := `[]`
			if scenario.service == "cutout" {
				assets = `["asset_input1"]`
			}
			body := fmt.Sprintf(`{"data":{"service":%q,"prompt":"a red bicycle","inputs":{"media_asset_ids":%s},"params":%s%s}}`, scenario.service, assets, scenario.params, scenario.quantity)
			status, payload := fx.postTask(t, "quantity-plan", body)
			if scenario.want == 0 {
				if status != http.StatusUnprocessableEntity || v1ErrorFields(t, payload)["quantity"] != "mismatch" && v1ErrorFields(t, payload)["quantity"] != "invalid" {
					t.Fatalf("submit = %d %v, want 422 quantity error", status, payload)
				}
				if fx.quoter.calls != 0 || len(fx.holder.holds) != 0 || len(fx.queue.enqueued) != 0 {
					t.Fatalf("rejected quantity reached Quote/Hold/enqueue: %d/%d/%d", fx.quoter.calls, len(fx.holder.holds), len(fx.queue.enqueued))
				}
				return
			}
			if status != http.StatusAccepted || len(fx.queue.enqueued) != 1 {
				t.Fatalf("submit = %d %v, want one accepted Task", status, payload)
			}
			created := fx.queue.enqueued[0]
			canonical := fx.canonicalParams(t, created.ID)
			plan, err := workflow.DecodeFeatureExecutionPlan(canonical.ExecutionPlan)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := created.BillingSnapshot
			if snapshot.Quantity != scenario.want || snapshot.Quantity != int64(plan.RequestedOutputCount().Int()) || snapshot.TotalCredits != 10*scenario.want || fx.holder.holds[snapshot.HoldTransactionID] != snapshot.TotalCredits {
				t.Fatalf("billing=%+v, plan count=%d, want quantity=%d with matching Hold", snapshot, plan.RequestedOutputCount().Int(), scenario.want)
			}
		})
	}
}

func TestV1TaskQuantityNormalizationPreservesReplay(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	const body = `{"data":{"service":"generation","prompt":"a red bicycle","params":{"output_count":4}%s}}`
	status, first := fx.postTask(t, "quantity-replay", fmt.Sprintf(body, ""))
	if status != http.StatusAccepted {
		t.Fatalf("first submit = %d %v", status, first)
	}
	status, second := fx.postTask(t, "quantity-replay", fmt.Sprintf(body, `,"quantity":4`))
	if status != http.StatusAccepted || v1Data(t, first).(map[string]any)["id"] != v1Data(t, second).(map[string]any)["id"] {
		t.Fatalf("normalized replay = %d %v, first=%v", status, second, first)
	}
	if fx.quoter.calls != 1 || len(fx.holder.holds) != 1 || len(fx.queue.enqueued) != 1 {
		t.Fatal("quantity replay repeated Quote/Hold/enqueue")
	}
}

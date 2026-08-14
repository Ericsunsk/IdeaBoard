package tasksubmission

import (
	"testing"

	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
)

func TestNewCanonicalRequestRejectsMismatchedBillingProduct(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:         42,
		Feature:        "style-variation",
		ClientActionID: "action-1",
		BillingRequest: &taskbilling.Request{
			Feature:  "prompt-generation",
			Quantity: 1,
		},
	})
	if err == nil {
		t.Fatal("expected mismatched workflow and Billing product to be rejected")
	}
}

func TestNewCanonicalRequestRejectsInvalidPriority(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:   42,
		Feature:  "style-variation",
		Priority: tasks.TaskPriorityLowest + 1,
	})
	if err == nil {
		t.Fatal("expected invalid priority to be rejected")
	}
}

func TestNewCanonicalRequestRejectsInvalidUserID(t *testing.T) {
	for _, userID := range []int64{0, -1} {
		_, err := NewCanonicalRequest(CanonicalRequestInput{
			UserID:  userID,
			Feature: "style-variation",
		})
		if err == nil {
			t.Fatalf("expected userID %d to be rejected", userID)
		}
	}
}

func TestNewCanonicalRequestRejectsMissingBillingForBillableWorkflow(t *testing.T) {
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:  42,
		Feature: "style-variation",
	})
	if err == nil {
		t.Fatal("expected billable workflow without Billing request to be rejected")
	}
}

func TestCanonicalRequestOwnsBillingFacts(t *testing.T) {
	inputBilling := &taskbilling.Request{Feature: "style-variation", Quantity: 1}
	request, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID:         42,
		Feature:        inputBilling.Feature,
		ClientActionID: "action-1",
		BillingRequest: inputBilling,
	})
	if err != nil {
		t.Fatal(err)
	}

	inputBilling.Feature = "prompt-generation"
	inputBilling.Quantity = 9
	billingRequest := request.BillingRequest()
	if billingRequest == nil || billingRequest.Feature != "style-variation" || billingRequest.Quantity != 1 {
		t.Fatalf("Billing request=%+v, want immutable style-variation quantity 1", billingRequest)
	}
}

func TestNewCanonicalRequestRejectsBillingQuantityMismatchedWithGeneration(t *testing.T) {
	count := 2
	_, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42, Feature: "style-variation", ClientActionID: "action-1",
		BillingRequest: &taskbilling.Request{Feature: "style-variation", Quantity: 1},
		Generation: &tasks.GenerationRequestSnapshot{
			Feature: "style-variation", Primitive: "text-to-image", TemplateFingerprint: "fp", RequestedOutputCount: count,
		},
	})
	if err == nil {
		t.Fatal("expected billing quantity to match generation output count")
	}
}

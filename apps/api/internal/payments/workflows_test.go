package payments

import "testing"

func TestWorkflowConstructorsRejectIncompleteDependencies(t *testing.T) {
	runtime := NewPaymentRuntime(nil, nil)
	if _, err := NewPaymentOrderTransitions(nil, runtime); err == nil {
		t.Fatal("NewPaymentOrderTransitions accepted nil store")
	}
	if _, err := NewCheckoutWorkflow(CheckoutWorkflowDependencies{}); err == nil {
		t.Fatal("NewCheckoutWorkflow accepted incomplete dependencies")
	}
	if _, err := NewStripeFactsWorkflow(StripeFactsWorkflowDependencies{}); err == nil {
		t.Fatal("NewStripeFactsWorkflow accepted incomplete dependencies")
	}
	if _, err := NewService(nil, nil); err == nil {
		t.Fatal("NewService accepted incomplete workflows")
	}
}

package payments_test

import (
	"ideaboard/api/internal/payments"
	"testing"
)

func TestWorkflowConstructorsRejectIncompleteDependencies(t *testing.T) {
	runtime := payments.NewPaymentRuntime(nil, nil)
	if _, err := payments.NewPaymentOrderTransitions(nil, runtime); err == nil {
		t.Fatal("NewPaymentOrderTransitions accepted nil store")
	}
	if _, err := payments.NewService(payments.ServiceDependencies{}); err == nil {
		t.Fatal("NewService accepted incomplete dependencies")
	}
	if _, err := payments.NewService(payments.ServiceDependencies{Transitions: nil}); err == nil {
		t.Fatal("NewService accepted missing transitions")
	}
}

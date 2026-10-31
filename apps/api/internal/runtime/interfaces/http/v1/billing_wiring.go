package v1

import (
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/billing"

	"github.com/gin-gonic/gin"
)

// ContractV1BillingDeps wires the workspace billing handlers.
type ContractV1BillingDeps = billing.Deps

// billingAccountStore resolves the stable funds identity plus financial management.
type billingAccountStore = billing.AccountStore

var NewPaymentsCheckoutAdapter = billing.NewPaymentsCheckoutAdapter

// RegisterContractV1BillingRoutesOnEngine mounts the workspace billing
// surface on its own /api/v1 group.
func RegisterContractV1BillingRoutesOnEngine(r *gin.Engine, authDeps runtimehttp.AuthDeps, deps ContractV1BillingDeps) {
	billing.RegisterRoutesOnEngine(r, authDeps, deps)
}

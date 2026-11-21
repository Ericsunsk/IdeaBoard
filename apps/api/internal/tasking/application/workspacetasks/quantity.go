package workspacetasks

import (
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

// ResolveSubmissionQuantity binds billing to the frozen execution output count.
// A nil request derives the quantity; an explicit request must agree with it.
// Uncompiled application inputs retain their existing quantity/default contract.
func ResolveSubmissionQuantity(canonicalParams string, requested *int64) (int64, error) {
	if requested != nil && *requested <= 0 {
		return 0, tasks.NewValidationError(tasks.FieldError{Field: "quantity", Message: "invalid"})
	}
	if canonicalParams == "" {
		if requested != nil {
			return *requested, nil
		}
		return 1, nil
	}
	params, err := tasks.UnmarshalParams(canonicalParams)
	if err != nil {
		return 0, tasks.NewValidationError(tasks.FieldError{Field: "params", Message: "invalid"})
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(params.ExecutionPlan)
	if err != nil {
		return 0, tasks.NewValidationError(tasks.FieldError{Field: "params", Message: "invalid"})
	}
	quantity := int64(plan.RequestedOutputCount().Int())
	if requested != nil && *requested != quantity {
		return 0, tasks.NewValidationError(tasks.FieldError{Field: "quantity", Message: "mismatch"})
	}
	return quantity, nil
}

package domain

const (
	StatusPending              = "pending"
	StatusRunning              = "running"
	StatusCompleted            = "completed"
	StatusFailed               = "failed"
	StatusCancelled            = "cancelled"
	StatusCancelledWithResults = "cancelled_with_results"
)

const ResultStatusSuccess = "success"

func IsTerminalStatus(status string) bool {
	return status == StatusCompleted || status == StatusFailed || status == StatusCancelled || status == StatusCancelledWithResults
}

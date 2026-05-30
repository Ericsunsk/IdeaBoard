package tasks

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const ResultStatusSuccess = "success"

func IsTerminalStatus(status string) bool {
	return status == StatusCompleted || status == StatusFailed
}

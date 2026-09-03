package reconciliation

import "time"

const (
	Interval      = 5 * time.Minute
	OperableDelay = time.Hour
	AnomalyAge    = 24 * time.Hour
	Lease         = 5 * time.Minute
)

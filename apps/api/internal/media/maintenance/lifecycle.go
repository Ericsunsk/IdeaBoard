package maintenance

import "time"

const (
	BatchSize           int32         = 100
	StaleUploadingAfter time.Duration = 30 * time.Minute
	ProcessingLease     time.Duration = 15 * time.Minute
	UploadingLease      time.Duration = 30 * time.Minute
	OrphanScanInterval  time.Duration = 15 * time.Minute
)

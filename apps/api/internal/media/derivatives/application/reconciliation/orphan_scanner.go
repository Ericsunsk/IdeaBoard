package reconciliation

import (
	"context"

	storagedomain "ideaboard/api/internal/media/storage/domain"
)

type DerivativeOrphanFinding struct {
	StorageProfileID string
	Count            int
}

type DerivativeOrphanScanFailure struct {
	StorageProfileID string
	Err              error
}

type DerivativeOrphanScanResult struct {
	Findings []DerivativeOrphanFinding
	Failures []DerivativeOrphanScanFailure
}

func ScanDerivativeOrphanObjects(
	ctx context.Context,
	profiles []storagedomain.StorageProfile,
	expected map[string]map[string]struct{},
	listObjectKeys func(context.Context, string, string) ([]string, error),
) DerivativeOrphanScanResult {
	result := DerivativeOrphanScanResult{}
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		keys, err := listObjectKeys(ctx, profile.ID, "uploads/")
		if err != nil {
			result.Failures = append(result.Failures, DerivativeOrphanScanFailure{
				StorageProfileID: profile.ID,
				Err:              err,
			})
			continue
		}
		orphanCount := 0
		for _, key := range keys {
			if !storagedomain.IsImageDerivativeObjectKey(key) {
				continue
			}
			if _, ok := expected[profile.ID][key]; ok {
				continue
			}
			orphanCount++
		}
		if orphanCount > 0 {
			result.Findings = append(result.Findings, DerivativeOrphanFinding{
				StorageProfileID: profile.ID,
				Count:            orphanCount,
			})
		}
	}
	return result
}

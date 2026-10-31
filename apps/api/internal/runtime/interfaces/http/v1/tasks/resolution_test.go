package tasks

import (
	"testing"
)

func TestReconcileTaskResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		topResolution  string
		planResolution any
		wantCanonical  string
		wantValid      bool
		wantConflict   bool
	}{
		{
			name:           "both empty",
			topResolution:  "",
			planResolution: nil,
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   false,
		},
		{
			name:           "top valid, plan nil",
			topResolution:  "2K",
			planResolution: nil,
			wantCanonical:  "2k",
			wantValid:      true,
			wantConflict:   false,
		},
		{
			name:           "top invalid, plan nil",
			topResolution:  "8k",
			planResolution: nil,
			wantCanonical:  "",
			wantValid:      false,
			wantConflict:   false,
		},
		{
			name:           "top empty, plan valid string",
			topResolution:  "",
			planResolution: "4K",
			wantCanonical:  "4k",
			wantValid:      true,
			wantConflict:   false,
		},
		{
			name:           "top empty, plan invalid string",
			topResolution:  "",
			planResolution: "default",
			wantCanonical:  "",
			wantValid:      false,
			wantConflict:   false,
		},
		{
			name:           "top empty, plan non-string",
			topResolution:  "",
			planResolution: 1080,
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   false,
		},
		{
			name:           "top empty, plan whitespace",
			topResolution:  "",
			planResolution: "   ",
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   false,
		},
		{
			name:           "top non-empty, plan non-string mismatch",
			topResolution:  "2k",
			planResolution: 1080,
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   true,
		},
		{
			name:           "top non-empty, plan empty string mismatch",
			topResolution:  "2k",
			planResolution: " ",
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   true,
		},
		{
			name:           "top and plan agree",
			topResolution:  "2K",
			planResolution: "2k",
			wantCanonical:  "2k",
			wantValid:      true,
			wantConflict:   false,
		},
		{
			name:           "top and plan disagree",
			topResolution:  "4k",
			planResolution: "2k",
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   true,
		},
		{
			name:           "top invalid, plan valid",
			topResolution:  "invalid-res",
			planResolution: "2k",
			wantCanonical:  "",
			wantValid:      true,
			wantConflict:   true,
		},
		{
			name:           "top valid, plan invalid",
			topResolution:  "2k",
			planResolution: "invalid-plan",
			wantCanonical:  "",
			wantValid:      false,
			wantConflict:   false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotCanonical, gotValid, gotConflict := reconcileTaskResolution(tc.topResolution, tc.planResolution)
			if gotCanonical != tc.wantCanonical || gotValid != tc.wantValid || gotConflict != tc.wantConflict {
				t.Errorf("reconcileTaskResolution(%q, %#v) = (%q, %v, %v); want (%q, %v, %v)",
					tc.topResolution, tc.planResolution,
					gotCanonical, gotValid, gotConflict,
					tc.wantCanonical, tc.wantValid, tc.wantConflict,
				)
			}
		})
	}
}

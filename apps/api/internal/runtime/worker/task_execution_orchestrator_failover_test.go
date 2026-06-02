package worker

import (
	"reflect"
	"testing"
)

func TestRemoveAccountCandidate(t *testing.T) {
	tests := []struct {
		name      string
		accountID int64
		input     []int64
		want      []int64
	}{
		{
			name:      "remove middle account",
			accountID: 2,
			input:     []int64{1, 2, 3},
			want:      []int64{1, 3},
		},
		{
			name:      "remove last account leaves exhausted pool",
			accountID: 1,
			input:     []int64{1},
			want:      []int64{},
		},
		{
			name:      "missing account keeps pool unchanged",
			accountID: 9,
			input:     []int64{1, 2},
			want:      []int64{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeAccountCandidate(tt.input, tt.accountID)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("removeAccountCandidate(%v, %d)=%v, want=%v", tt.input, tt.accountID, got, tt.want)
			}
		})
	}
}

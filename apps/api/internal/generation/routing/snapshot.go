package routing

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
)

// SelectionSeed is a 128-bit cryptographically random per-task seed
// generated at submit and pinned in the snapshot (ADR-0012 rule 5). It
// renders as a 32-character lowercase hex string in JSON so the
// task-row JSONB snapshot stays human-diagnosable.
type SelectionSeed [16]byte

// NewSelectionSeed mints one 128-bit cryptographically random seed. A
// randomness failure is a hard error: selection must never fall back to
// a predictable seed.
func NewSelectionSeed() (SelectionSeed, error) {
	var seed SelectionSeed
	if _, err := rand.Read(seed[:]); err != nil {
		return SelectionSeed{}, fmt.Errorf("routing: mint selection seed: %w", err)
	}
	return seed, nil
}

// String renders the seed as lowercase hex.
func (s SelectionSeed) String() string {
	return hex.EncodeToString(s[:])
}

// MarshalJSON renders the seed as a 32-character lowercase hex string.
func (s SelectionSeed) MarshalJSON() ([]byte, error) {
	dst := make([]byte, 2+hex.EncodedLen(len(s)))
	dst[0] = '"'
	hex.Encode(dst[1:], s[:])
	dst[len(dst)-1] = '"'
	return dst, nil
}

// UnmarshalJSON parses the 32-character lowercase (or upper) hex form.
// Any other shape is rejected so corrupt snapshots fail closed.
func (s *SelectionSeed) UnmarshalJSON(data []byte) error {
	if len(data) != 2+2*len(s) || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("routing: invalid selection seed %q", string(data))
	}
	if _, err := hex.Decode(s[:], data[1:len(data)-1]); err != nil {
		return fmt.Errorf("routing: invalid selection seed %q: %w", string(data), err)
	}
	return nil
}

// RoutingSnapshot is the immutable submit-time routing fact pinned per
// task (ADR-0012 rules 2 and 9). Field names are the shared contract;
// JSON names are the task-row JSONB shape wave 2 persists. The snapshot
// is sufficient to replay and diagnose selection plus fallback without
// the live pool: Verify recomputes the decision from these fields
// alone.
type RoutingSnapshot struct {
	Service             string     `json:"service"`
	RequestedResolution Resolution `json:"requested_resolution"`
	EligibleModels      []string   `json:"eligible_models"`
	SelectedModel       string     `json:"selected_model"`
	// RetryCandidate is the next sorted candidate after SelectedModel
	// (wrapping), or empty when only one candidate exists. It is the
	// only model fallback may try (ADR-0012 rule 6).
	RetryCandidate string        `json:"retry_candidate"`
	SelectionSeed  SelectionSeed `json:"selection_seed"`
	PoolVersion    uint64        `json:"pool_version"`
}

// HasRetryCandidate reports whether a bounded model fallback is
// possible: a retryable failure may try the pinned candidate exactly
// once, and only when one exists.
func (s RoutingSnapshot) HasRetryCandidate() bool {
	return s.RetryCandidate != ""
}

// Verify replays the selection from the snapshot fields alone and
// reports whether the pinned decision matches. It needs no live pool:
// later admin pool edits never invalidate an accepted task nor change
// its candidate set (ADR-0012 rule 9).
func (s RoutingSnapshot) Verify() error {
	if len(s.EligibleModels) == 0 {
		return fmt.Errorf("routing: snapshot has no eligible models")
	}
	if !sort.StringsAreSorted(s.EligibleModels) {
		return fmt.Errorf("routing: snapshot eligible models are not sorted")
	}
	selected, retry := SelectCandidate(s.SelectionSeed, s.PoolVersion, s.Service, s.RequestedResolution, s.EligibleModels)
	if selected != s.SelectedModel || retry != s.RetryCandidate {
		return fmt.Errorf("routing: snapshot selection mismatch: have %q/%q, replay gives %q/%q",
			s.SelectedModel, s.RetryCandidate, selected, retry)
	}
	return nil
}

// SelectCandidate is the frozen selection algorithm (ADR-0012 rule 5,
// execution-guide frozen decisions). Eligible canonical model
// references are sorted lexicographically; the selected index is
//
//	SHA-256(selection_seed || U64BE(pool_version) || 0x00 ||
//	        UTF8(service) || 0x00 || UTF8(resolution))
//
// modulo the sorted candidate count, computed over the full 256-bit
// digest so the remainder is uniform across independent seeds. The next
// sorted candidate wraps as the single retry candidate, or is empty
// when only one candidate exists.
//
// Callers must pass the normalized service and resolution and must not
// mutate eligible; the slice is copied before sorting.
func SelectCandidate(seed SelectionSeed, poolVersion uint64, service string, resolution Resolution, eligible []string) (selected, retry string) {
	sorted := make([]string, len(eligible))
	copy(sorted, eligible)
	sort.Strings(sorted)

	h := sha256.New()
	h.Write(seed[:])
	var versionBytes [8]byte
	binary.BigEndian.PutUint64(versionBytes[:], poolVersion)
	h.Write(versionBytes[:])
	h.Write([]byte{0x00})
	h.Write([]byte(service))
	h.Write([]byte{0x00})
	h.Write([]byte(string(resolution)))
	sum := h.Sum(nil)

	n := len(sorted)
	if n == 0 {
		return "", ""
	}
	index := new(big.Int).SetBytes(sum)
	index.Mod(index, big.NewInt(int64(n)))
	selected = sorted[index.Int64()]
	if n > 1 {
		retry = sorted[(index.Int64()+1)%int64(n)]
	}
	return selected, retry
}

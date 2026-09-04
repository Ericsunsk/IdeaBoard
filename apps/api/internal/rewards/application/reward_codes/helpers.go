package reward_codes

import (
	"ideaboard/api/internal/rewards/domain"
)

// Re-exported error sentinels for callers to import from a single package.
var (
	ErrInvalidCommand      = domain.ErrInvalidCode
	ErrCodeCollision       = domain.ErrCodeCollision
	ErrRewardCodeConflict  = domain.ErrRewardCodeConflict
	ErrCodeUnavailable     = domain.ErrCodeUnavailable
	ErrRewardCodeNotFound  = domain.ErrRewardCodeNotFound
	ErrCapBelowRedemptions = domain.ErrCapBelowRedemptions
	ErrRewardCodeRedeemed  = domain.ErrRewardCodeRedeemed
	ErrRedemptionNotFound  = domain.ErrRedemptionNotFound
)

// codeDigest is an alias to the domain-level code digest type used internally.
type codeDigest = domain.CodeDigest

// splitCodeDigestCandidates returns the current and previous digests, where
// the previous is zero when only one key version is active.
func splitCodeDigestCandidates(candidates []codeDigest) (codeDigest, codeDigest) {
	current := candidates[0]
	if len(candidates) == 1 {
		return current, codeDigest{}
	}
	return current, candidates[1]
}

const maskedCodeHintPrefix = "•••• "

package rewardcodes

import (
	"ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
)

type CreateRewardCodeCommand = ports.CreateRewardCodeCommand
type CreatedRewardCode = ports.CreatedRewardCode
type UpdateRewardCodeCommand = ports.UpdateRewardCodeCommand
type UpdatedRewardCode = ports.UpdatedRewardCode
type SetRewardCodeEnabledCommand = ports.SetRewardCodeEnabledCommand
type DeleteRewardCodeCommand = ports.DeleteRewardCodeCommand
type DeletedRewardCode = ports.DeletedRewardCode
type SearchRewardCodesInput = ports.SearchRewardCodesInput
type RewardCodePage = ports.RewardCodePage
type ListAdminRedemptionsInput = ports.ListAdminRedemptionsInput
type AdminRedemptionPage = ports.AdminRedemptionPage
type AdminRedemptionSummary = ports.AdminRedemptionSummary
type GetAdminRedemptionInput = ports.GetAdminRedemptionInput
type AdminRedemptionDetail = ports.AdminRedemptionDetail
type RewardCode = ports.RewardCode
type RewardCodeStatus = ports.RewardCodeStatus

const (
	RewardCodeStatusExpired   = ports.RewardCodeStatusExpired
	RewardCodeStatusExhausted = ports.RewardCodeStatusExhausted
	RewardCodeStatusDisabled  = ports.RewardCodeStatusDisabled
	RewardCodeStatusEnabled   = ports.RewardCodeStatusEnabled
)

type CodeKeyring = domain.CodeKeyring

// merged from internal/rewards/application/rewardcodes/helpers.go (structural consolidation; symbols unchanged).
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

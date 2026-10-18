package redemptions

import (
	"ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
)

var (
	ErrInvalidCommand     = domain.ErrInvalidCode
	ErrCodeUnavailable    = domain.ErrCodeUnavailable
	ErrRewardCodeNotFound = domain.ErrRewardCodeNotFound
)

type RedeemCommand = ports.RedeemCommand
type RedemptionHistoryItem = ports.RedemptionHistoryItem
type RedemptionHistoryPage = ports.RedemptionHistoryPage
type RedeemResult = ports.RedeemResult
type Redemption = ports.Redemption
type ListRedemptionsInput = ports.ListRedemptionsInput
type RewardCode = ports.RewardCode

// merged from internal/rewards/application/redemptions/helpers.go (structural consolidation; symbols unchanged).
type CodeDigest = domain.CodeDigest
type CodeKeyring = domain.CodeKeyring

func splitCodeDigestCandidates(candidates []CodeDigest) (CodeDigest, CodeDigest) {
	current := candidates[0]
	if len(candidates) == 1 {
		return current, CodeDigest{}
	}
	return current, candidates[1]
}

package redemptions

import (
	"ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
)

const maskedCodeHintPrefix = "•••• "

type Status = ports.RewardCodeStatus

const (
	StatusExpired   Status = ports.RewardCodeStatusExpired
	StatusExhausted Status = ports.RewardCodeStatusExhausted
	StatusDisabled  Status = ports.RewardCodeStatusDisabled
	StatusEnabled   Status = ports.RewardCodeStatusEnabled
)

type CodeDigest = domain.CodeDigest
type CodeKeyring = domain.CodeKeyring

func splitCodeDigestCandidates(candidates []CodeDigest) (CodeDigest, CodeDigest) {
	current := candidates[0]
	if len(candidates) == 1 {
		return current, CodeDigest{}
	}
	return current, candidates[1]
}

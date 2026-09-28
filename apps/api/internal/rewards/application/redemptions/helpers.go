package redemptions

import (
	"ideaboard/api/internal/rewards/domain"
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

package reward_codes

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

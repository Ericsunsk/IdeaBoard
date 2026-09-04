package rewards

import (
	"ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
)

var (
	ErrInvalidCommand      = domain.ErrInvalidCode
	ErrCodeCollision       = domain.ErrCodeCollision
	ErrRewardCodeConflict  = domain.ErrRewardCodeConflict
	ErrCodeUnavailable     = domain.ErrCodeUnavailable
	ErrRewardCodeNotFound  = domain.ErrRewardCodeNotFound
	ErrCapBelowRedemptions = domain.ErrCapBelowRedemptions
	ErrRewardCodeRedeemed  = domain.ErrRewardCodeRedeemed
	ErrRedemptionNotFound  = domain.ErrRedemptionNotFound
	ErrWalletFrozen        = domain.ErrWalletFrozen
)

type RewardCodeStatus = ports.RewardCodeStatus

const (
	RewardCodeStatusExpired   = ports.RewardCodeStatusExpired
	RewardCodeStatusExhausted = ports.RewardCodeStatusExhausted
	RewardCodeStatusDisabled  = ports.RewardCodeStatusDisabled
	RewardCodeStatusEnabled   = ports.RewardCodeStatusEnabled
)

type RewardCode = ports.RewardCode
type CreatedRewardCode = ports.CreatedRewardCode
type CreateRewardCodeCommand = ports.CreateRewardCodeCommand
type UpdateRewardCodeCommand = ports.UpdateRewardCodeCommand
type UpdatedRewardCode = ports.UpdatedRewardCode
type SetRewardCodeEnabledCommand = ports.SetRewardCodeEnabledCommand
type DeleteRewardCodeCommand = ports.DeleteRewardCodeCommand
type DeletedRewardCode = ports.DeletedRewardCode
type SearchRewardCodesInput = ports.SearchRewardCodesInput
type RewardCodePage = ports.RewardCodePage
type AdminRedemptionSummary = ports.AdminRedemptionSummary
type ListAdminRedemptionsInput = ports.ListAdminRedemptionsInput
type AdminRedemptionPage = ports.AdminRedemptionPage
type GetAdminRedemptionInput = ports.GetAdminRedemptionInput
type AdminRedemptionDetail = ports.AdminRedemptionDetail
type RedeemCommand = ports.RedeemCommand
type Redemption = ports.Redemption
type RedeemResult = ports.RedeemResult
type RedemptionHistoryItem = ports.RedemptionHistoryItem
type ListRedemptionsInput = ports.ListRedemptionsInput
type RedemptionHistoryPage = ports.RedemptionHistoryPage

type AdminCommands = ports.AdminCommands
type AdminReads = ports.AdminReads
type UserCommands = ports.UserCommands
type UserReads = ports.UserReads
type UserService = ports.UserService
type AdminCommandService = ports.AdminCommandService
type AdminReadService = ports.AdminReadService

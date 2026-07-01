package accounts

import "ideaboard/api/internal/platform/accountlimits"

func CurrentAccountLimits() AccountLimits {
	return AccountLimits{
		MaxConcurrent: AccountLimitRange{
			Min:     accountlimits.MinMaxConcurrent,
			Max:     accountlimits.MaxMaxConcurrent,
			Default: accountlimits.DefaultMaxConcurrent,
		},
		TimeoutSeconds: AccountLimitRange{
			Min:     accountlimits.MinTimeoutSeconds,
			Max:     accountlimits.MaxTimeoutSeconds,
			Default: accountlimits.DefaultTimeoutSeconds,
		},
	}
}

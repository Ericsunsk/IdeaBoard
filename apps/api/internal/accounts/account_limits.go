package accounts

import "ideaboard/api/internal/platform/accountlimits"

const minAccountMaxConcurrent = accountlimits.MinMaxConcurrent
const minAccountTimeoutSeconds = 1

func normalizeMaxConcurrent(value int) int {
	return accountlimits.NormalizeMaxConcurrent(value)
}

func NormalizeAccountTimeout(value int) int {
	if value < minAccountTimeoutSeconds {
		return defaultAccountTimeoutSeconds
	}
	if value > maxAccountTimeoutSeconds {
		return maxAccountTimeoutSeconds
	}
	return value
}

func CurrentAccountLimits() AccountLimits {
	return AccountLimits{
		MaxConcurrent: AccountLimitRange{
			Min:     minAccountMaxConcurrent,
			Max:     maxAccountMaxConcurrent,
			Default: defaultAccountMaxConcurrent,
		},
		TimeoutSeconds: AccountLimitRange{
			Min:     minAccountTimeoutSeconds,
			Max:     maxAccountTimeoutSeconds,
			Default: defaultAccountTimeoutSeconds,
		},
	}
}

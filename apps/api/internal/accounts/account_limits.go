package accounts

const minAccountMaxConcurrent = 1
const minAccountTimeoutSeconds = 1

func normalizeMaxConcurrent(value int) int {
	if value < minAccountMaxConcurrent {
		return defaultAccountMaxConcurrent
	}
	if value > maxAccountMaxConcurrent {
		return maxAccountMaxConcurrent
	}
	return value
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

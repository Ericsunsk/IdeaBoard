package accounts

func normalizeMaxConcurrent(value int) int {
	return NormalizeMaxConcurrent(value)
}

func NormalizeMaxConcurrent(value int) int {
	if value <= 0 {
		return defaultAccountMaxConcurrent
	}
	if value > maxAccountMaxConcurrent {
		return maxAccountMaxConcurrent
	}
	return value
}

func NormalizeAccountTimeout(value int) int {
	if value <= 0 {
		return defaultAccountTimeoutSeconds
	}
	if value > maxAccountTimeoutSeconds {
		return maxAccountTimeoutSeconds
	}
	return value
}

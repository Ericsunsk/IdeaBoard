package accountlimits

const (
	MinMaxConcurrent     = 1
	DefaultMaxConcurrent = 3
	MaxMaxConcurrent     = 64

	MinTimeoutSeconds     = 1
	DefaultTimeoutSeconds = 120
	MaxTimeoutSeconds     = 600
)

func NormalizeMaxConcurrent(value int) int {
	if value < MinMaxConcurrent {
		return DefaultMaxConcurrent
	}
	if value > MaxMaxConcurrent {
		return MaxMaxConcurrent
	}
	return value
}

func NormalizeTimeoutSeconds(value int) int {
	if value < MinTimeoutSeconds {
		return DefaultTimeoutSeconds
	}
	if value > MaxTimeoutSeconds {
		return MaxTimeoutSeconds
	}
	return value
}

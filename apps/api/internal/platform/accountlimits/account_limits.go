package accountlimits

const (
	MinMaxConcurrent     = 1
	DefaultMaxConcurrent = 3
	MaxMaxConcurrent     = 64
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

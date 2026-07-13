package usage

import (
	"errors"
	"fmt"
)

type Range string

const (
	Range7Days  Range = "7d"
	Range30Days Range = "30d"
	Range1Year  Range = "1y"

	DefaultRange = Range30Days
)

var ErrInvalidRange = errors.New("unsupported usage range")

func ParseRange(raw string) (Range, error) {
	if raw == "" {
		return DefaultRange, nil
	}
	usageRange := Range(raw)
	if !usageRange.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidRange, raw)
	}
	return usageRange, nil
}

func (r Range) Valid() bool {
	switch r {
	case Range7Days, Range30Days, Range1Year:
		return true
	default:
		return false
	}
}

func (r Range) Days() int {
	switch r {
	case Range7Days:
		return 7
	case Range30Days:
		return 30
	case Range1Year:
		return 365
	default:
		return 0
	}
}

package workflow

import "fmt"

type GenerationPrimitive string

const (
	GenerationPrimitiveTextToImage  GenerationPrimitive = "text-to-image"
	GenerationPrimitiveImageToImage GenerationPrimitive = "image-to-image"
)

func ParseGenerationPrimitive(raw string) (GenerationPrimitive, error) {
	primitive := GenerationPrimitive(raw)
	switch primitive {
	case GenerationPrimitiveTextToImage, GenerationPrimitiveImageToImage:
		return primitive, nil
	default:
		return "", fmt.Errorf("generation primitive %q is invalid", raw)
	}
}

type RequestedOutputCount uint8

const (
	MinRequestedOutputCount = 1
	MaxRequestedOutputCount = 4
)

func ParseRequestedOutputCount(value int) (RequestedOutputCount, error) {
	if value < MinRequestedOutputCount || value > MaxRequestedOutputCount {
		return 0, fmt.Errorf("requested output count must be between %d and %d", MinRequestedOutputCount, MaxRequestedOutputCount)
	}
	return RequestedOutputCount(value), nil
}

func (count RequestedOutputCount) Int() int { return int(count) }

func (count RequestedOutputCount) Validate() error {
	_, err := ParseRequestedOutputCount(count.Int())
	return err
}

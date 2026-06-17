package boards

import (
	"encoding/json"
	"math"
	"strings"
)

const CurrentBoardStateSchemaVersion = 1

// BoardStateEnvelope validates the outer canvas state contract while leaving
// node internals to the frontend canvas schema.
type BoardStateEnvelope struct {
	SchemaVersion int               `json:"schemaVersion"`
	ImageNodes    []json.RawMessage `json:"imageNodes"`
	PromptNodes   []json.RawMessage `json:"promptNodes"`
	FusionNodes   []json.RawMessage `json:"fusionNodes"`
	Connections   []json.RawMessage `json:"connections"`
	Viewport      json.RawMessage   `json:"viewport"`
	MediaRefs     []json.RawMessage `json:"mediaRefs,omitempty"`
}

// ParseBoardStateEnvelope extracts the schema version from raw state JSON for
// read-side metadata.
func ParseBoardStateEnvelope(raw string) BoardStateEnvelope {
	if strings.TrimSpace(raw) == "" {
		return BoardStateEnvelope{}
	}
	env, err := validateBoardStateEnvelopeJSON([]byte(raw))
	if err != nil {
		return BoardStateEnvelope{}
	}
	return env
}

// StateSchemaVersion returns the schema version from a stored UserBoard row.
func StateSchemaVersion(board *UserBoard) int {
	if board == nil {
		return 0
	}
	if board.StateSchemaVersion > 0 {
		return int(board.StateSchemaVersion)
	}
	return ParseBoardStateEnvelope(board.State).SchemaVersion
}

func validateBoardStateEnvelopeJSON(payload []byte) (BoardStateEnvelope, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil || raw == nil {
		return BoardStateEnvelope{}, ErrInvalidBoardInput
	}

	schemaVersion, err := requiredIntegerField(raw, "schemaVersion")
	if err != nil || schemaVersion != CurrentBoardStateSchemaVersion {
		return BoardStateEnvelope{}, ErrInvalidBoardInput
	}

	imageNodes, err := requiredArrayField(raw, "imageNodes")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	promptNodes, err := requiredArrayField(raw, "promptNodes")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	fusionNodes, err := requiredArrayField(raw, "fusionNodes")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	connections, err := requiredArrayField(raw, "connections")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	viewport, err := requiredObjectField(raw, "viewport")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	if err := validateBoardStateViewport(viewport); err != nil {
		return BoardStateEnvelope{}, err
	}
	if _, err := requiredNumberField(raw, "updatedAt"); err != nil {
		return BoardStateEnvelope{}, err
	}
	mediaRefs, err := optionalArrayField(raw, "mediaRefs")
	if err != nil {
		return BoardStateEnvelope{}, err
	}

	return BoardStateEnvelope{
		SchemaVersion: schemaVersion,
		ImageNodes:    imageNodes,
		PromptNodes:   promptNodes,
		FusionNodes:   fusionNodes,
		Connections:   connections,
		Viewport:      viewport,
		MediaRefs:     mediaRefs,
	}, nil
}

func validateBoardStateViewport(raw json.RawMessage) error {
	var viewport map[string]json.RawMessage
	if err := json.Unmarshal(raw, &viewport); err != nil || viewport == nil {
		return ErrInvalidBoardInput
	}
	for _, key := range []string{"canvasX", "canvasY", "zoomLevel"} {
		if _, err := requiredNumberField(viewport, key); err != nil {
			return err
		}
	}
	return nil
}

func requiredIntegerField(raw map[string]json.RawMessage, key string) (int, error) {
	value, err := requiredNumberField(raw, key)
	if err != nil {
		return 0, err
	}
	if value != math.Trunc(value) {
		return 0, ErrInvalidBoardInput
	}
	return int(value), nil
}

func requiredNumberField(raw map[string]json.RawMessage, key string) (float64, error) {
	value, ok := raw[key]
	if !ok {
		return 0, ErrInvalidBoardInput
	}
	var number float64
	if err := json.Unmarshal(value, &number); err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, ErrInvalidBoardInput
	}
	return number, nil
}

func requiredArrayField(raw map[string]json.RawMessage, key string) ([]json.RawMessage, error) {
	value, ok := raw[key]
	if !ok {
		return nil, ErrInvalidBoardInput
	}
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err != nil || items == nil {
		return nil, ErrInvalidBoardInput
	}
	return items, nil
}

func optionalArrayField(raw map[string]json.RawMessage, key string) ([]json.RawMessage, error) {
	if _, ok := raw[key]; !ok {
		return nil, nil
	}
	return requiredArrayField(raw, key)
}

func requiredObjectField(raw map[string]json.RawMessage, key string) (json.RawMessage, error) {
	value, ok := raw[key]
	if !ok {
		return nil, ErrInvalidBoardInput
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return nil, ErrInvalidBoardInput
	}
	return value, nil
}

package boards

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
)

const CurrentBoardStateSchemaVersion = 3

var (
	boardStateGroupIDPattern     = regexp.MustCompile(`^group-[0-9]+$`)
	boardStateGroupNodeIDPattern = regexp.MustCompile(`^(image|prompt|fusion)-[0-9]+(?:\.[0-9]+)?$`)
)

var allowedBoardStateEnvelopeFields = map[string]struct{}{
	"schemaVersion":            {},
	"imageNodes":               {},
	"promptNodes":              {},
	"fusionNodes":              {},
	"connections":              {},
	"groups":                   {},
	"mediaRefs":                {},
	"previewSnapshotDataUrl":   {},
	"previewSnapshotUpdatedAt": {},
	"previewSnapshotVersion":   {},
	"updatedAt":                {},
}

// BoardStateEnvelope validates the outer canvas state contract while leaving
// node internals to the frontend canvas schema.
type BoardStateEnvelope struct {
	SchemaVersion int                  `json:"schemaVersion"`
	ImageNodes    []json.RawMessage    `json:"imageNodes"`
	PromptNodes   []json.RawMessage    `json:"promptNodes"`
	FusionNodes   []json.RawMessage    `json:"fusionNodes"`
	Connections   []json.RawMessage    `json:"connections"`
	Groups        []json.RawMessage    `json:"groups"`
	MediaRefs     []BoardStateMediaRef `json:"mediaRefs,omitempty"`
}

type BoardStateMediaRef struct {
	MediaAssetID int64  `json:"mediaAssetId"`
	RefType      string `json:"refType"`
	RefID        string `json:"refId"`
	Role         string `json:"role,omitempty"`
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
	for key := range raw {
		if _, ok := allowedBoardStateEnvelopeFields[key]; !ok {
			return BoardStateEnvelope{}, ErrInvalidBoardInput
		}
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
	groups, err := requiredArrayField(raw, "groups")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	if err := validateBoardStateGroups(groups); err != nil {
		return BoardStateEnvelope{}, err
	}
	if _, err := requiredNumberField(raw, "updatedAt"); err != nil {
		return BoardStateEnvelope{}, err
	}
	mediaRefs, err := optionalMediaRefsField(raw, "mediaRefs")
	if err != nil {
		return BoardStateEnvelope{}, err
	}
	if _, err := optionalStringField(raw, "previewSnapshotDataUrl"); err != nil {
		return BoardStateEnvelope{}, err
	}
	if _, err := optionalNumberField(raw, "previewSnapshotUpdatedAt"); err != nil {
		return BoardStateEnvelope{}, err
	}
	if _, err := optionalPositiveIntegerField(raw, "previewSnapshotVersion"); err != nil {
		return BoardStateEnvelope{}, err
	}

	return BoardStateEnvelope{
		SchemaVersion: schemaVersion,
		ImageNodes:    imageNodes,
		PromptNodes:   promptNodes,
		FusionNodes:   fusionNodes,
		Connections:   connections,
		Groups:        groups,
		MediaRefs:     mediaRefs,
	}, nil
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

func optionalNumberField(raw map[string]json.RawMessage, key string) (float64, error) {
	if _, ok := raw[key]; !ok {
		return 0, nil
	}
	return requiredNumberField(raw, key)
}

func optionalPositiveIntegerField(raw map[string]json.RawMessage, key string) (int, error) {
	if _, ok := raw[key]; !ok {
		return 0, nil
	}
	value, err := requiredIntegerField(raw, key)
	if err != nil || value < 1 {
		return 0, ErrInvalidBoardInput
	}
	return value, nil
}

func optionalStringField(raw map[string]json.RawMessage, key string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil || text == "" {
		return "", ErrInvalidBoardInput
	}
	return text, nil
}

func optionalMediaRefsField(raw map[string]json.RawMessage, key string) ([]BoardStateMediaRef, error) {
	items, err := optionalArrayField(raw, key)
	if err != nil || items == nil {
		return nil, err
	}
	refs := make([]BoardStateMediaRef, 0, len(items))
	for _, item := range items {
		ref, err := validateBoardStateMediaRef(item)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func validateBoardStateGroups(items []json.RawMessage) error {
	for _, item := range items {
		if err := validateBoardStateGroup(item); err != nil {
			return err
		}
	}
	return nil
}

func validateBoardStateGroup(payload json.RawMessage) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil || raw == nil {
		return ErrInvalidBoardInput
	}
	for key := range raw {
		if key != "id" && key != "nodeIds" && key != "x" && key != "y" && key != "width" && key != "height" {
			return ErrInvalidBoardInput
		}
	}
	id, err := requiredStringField(raw, "id")
	if err != nil || !boardStateGroupIDPattern.MatchString(id) {
		return ErrInvalidBoardInput
	}
	nodeIDs, err := requiredStringArrayField(raw, "nodeIds")
	if err != nil {
		return err
	}
	for _, nodeID := range nodeIDs {
		if !boardStateGroupNodeIDPattern.MatchString(nodeID) {
			return ErrInvalidBoardInput
		}
	}
	if _, err := requiredNumberField(raw, "x"); err != nil {
		return err
	}
	if _, err := requiredNumberField(raw, "y"); err != nil {
		return err
	}
	width, err := requiredNumberField(raw, "width")
	if err != nil || width <= 0 {
		return ErrInvalidBoardInput
	}
	height, err := requiredNumberField(raw, "height")
	if err != nil || height <= 0 {
		return ErrInvalidBoardInput
	}
	return nil
}

func validateBoardStateMediaRef(payload json.RawMessage) (BoardStateMediaRef, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil || raw == nil {
		return BoardStateMediaRef{}, ErrInvalidBoardInput
	}
	for key := range raw {
		if key != "mediaAssetId" && key != "refType" && key != "refId" && key != "role" {
			return BoardStateMediaRef{}, ErrInvalidBoardInput
		}
	}
	mediaAssetID, err := requiredInt64Field(raw, "mediaAssetId")
	if err != nil {
		return BoardStateMediaRef{}, err
	}
	if mediaAssetID <= 0 {
		return BoardStateMediaRef{}, ErrInvalidBoardInput
	}
	refType, err := requiredStringField(raw, "refType")
	if err != nil {
		return BoardStateMediaRef{}, err
	}
	refType = strings.TrimSpace(refType)
	if refType != "board" && refType != "board_node" {
		return BoardStateMediaRef{}, ErrInvalidBoardInput
	}
	refID, err := requiredStringField(raw, "refId")
	if err != nil {
		return BoardStateMediaRef{}, err
	}
	refID = strings.TrimSpace(refID)
	if len(refID) > 128 {
		return BoardStateMediaRef{}, ErrInvalidBoardInput
	}
	role, err := optionalStringField(raw, "role")
	if err != nil {
		return BoardStateMediaRef{}, err
	}
	role = strings.TrimSpace(role)
	if len(role) > 32 {
		return BoardStateMediaRef{}, ErrInvalidBoardInput
	}
	return BoardStateMediaRef{
		MediaAssetID: mediaAssetID,
		RefType:      refType,
		RefID:        refID,
		Role:         role,
	}, nil
}

func requiredInt64Field(raw map[string]json.RawMessage, key string) (int64, error) {
	value, ok := raw[key]
	if !ok {
		return 0, ErrInvalidBoardInput
	}
	var number json.Number
	if err := json.Unmarshal(value, &number); err != nil {
		return 0, ErrInvalidBoardInput
	}
	parsed, err := number.Int64()
	if err != nil {
		return 0, ErrInvalidBoardInput
	}
	return parsed, nil
}

func requiredStringField(raw map[string]json.RawMessage, key string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", ErrInvalidBoardInput
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil || text == "" {
		return "", ErrInvalidBoardInput
	}
	return text, nil
}

func requiredStringArrayField(raw map[string]json.RawMessage, key string) ([]string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, ErrInvalidBoardInput
	}
	var items []string
	if err := json.Unmarshal(value, &items); err != nil || items == nil {
		return nil, ErrInvalidBoardInput
	}
	return items, nil
}

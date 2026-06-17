package boards

import "encoding/json"

// BoardStateEnvelope wraps the raw board state JSON with a typed schema version
// check. The full state body (nodes, edges, viewport, mediaRefs, etc.) remains
// opaque until the frontend canvas schema stabilises; this envelope provides a
// lightweight contract that lets the backend validate compatibility without
// deserialising every internal field.
type BoardStateEnvelope struct {
	SchemaVersion int `json:"schemaVersion"`
}

// ParseBoardStateEnvelope extracts the schema version from raw state JSON.
// Returns 0 for empty or unparseable state (backward-compatible).
func ParseBoardStateEnvelope(raw string) BoardStateEnvelope {
	if raw == "" {
		return BoardStateEnvelope{}
	}
	var env BoardStateEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return BoardStateEnvelope{}
	}
	return env
}

// StateSchemaVersion returns the schema version from a stored UserBoard row,
// falling back to parsing the raw state JSON if the dedicated column is zero.
func StateSchemaVersion(board *UserBoard) int {
	if board == nil {
		return 0
	}
	if board.StateSchemaVersion > 0 {
		return int(board.StateSchemaVersion)
	}
	return ParseBoardStateEnvelope(board.State).SchemaVersion
}

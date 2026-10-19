package boards

// Y.Doc collaboration (ADR-0020): the backend half of the Y.Doc sync plane.
//
// Document structure mirrors the TapNow benchmark (multiplayer architecture
// doc section 3): each board maps to one Y.Doc with two top-level maps,
// nodes and edges, both keyed by id. The realtime sidecar (apps/realtime)
// owns the live Doc: it applies client SyncStep2/Update frames, broadcasts
// them to room peers for WS incremental convergence, and persists the opaque
// update bytes through the Go internal surface. The Go side never decodes
// CRDT bytes.
//
// Two seams live here:
//
//   - BoardYUpdateStore.AppendYUpdate/ListYUpdates: the per-board update log
//     (board_y_updates). Appends require board CapWrite and a live board;
//     writers without it fail closed.
//   - BoardYUpdateStore.CompactYDocument: the snapshot compaction point. The
//     sidecar posts the Y-derived snapshot; it is sanitized (stripTransient
//     whitelist, illegal fields dropped) and validated with the same
//     ValidateV1DocumentStrict rules as the non-collaborator path, then lands
//     in board_documents with one appended board_revisions row in the same
//     transaction (the existing transactional pattern), and the update log is
//     trimmed. Collaborator compactions never take an expected revision and
//     never report 409: concurrent compactions serialize on the board row lock
//     and converge last-writer-wins. The public If-Match + 409 path
//     (ReplaceDocument) is untouched for non-collaborator writes.
//
// Hydration stays dual-channel: HTTP snapshot first (GET board + previewscene
// unchanged) with WS incremental convergence on top.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

const (
	// YDocNodesKey and YDocEdgesKey are the two top-level Y.Map names,
	// frozen by the benchmark contract (section 3).
	YDocNodesKey = "nodes"
	YDocEdgesKey = "edges"

	// MaxYUpdateBytes caps one persisted Y update. It mirrors the
	// chk_board_y_updates_data CHECK so oversized frames fail before SQL.
	MaxYUpdateBytes = 1 << 20
	// MaxYUpdatesListLimit caps ListYUpdates reads.
	MaxYUpdatesListLimit = int32(1000)
	// MaxYCompactEntries caps the total nodes+edges accepted in one
	// compaction snapshot. Overflow fails the compaction (400), it never
	// silently truncates content.
	MaxYCompactEntries = 10000
)

// YUpdateRecord is one persisted Y update (opaque CRDT bytes).
type YUpdateRecord struct {
	ID           int64
	AuthorUserID int64
	Update       []byte
	CreatedAt    time.Time
}

// BoardYUpdateStore is the Y.Doc persistence seam. Both WorkspaceBoardStore
// implementations satisfy it; the internal realtime handler requires it on
// the same board store so no optional persistence path exists.
type BoardYUpdateStore interface {
	AppendYUpdate(ctx context.Context, workspacePublicID, boardPublicID string, authorUserID int64, update []byte) (int64, error)
	ListYUpdates(ctx context.Context, workspacePublicID, boardPublicID string, limit int32) ([]*YUpdateRecord, error)
	CompactYDocument(ctx context.Context, workspacePublicID, boardPublicID string, authorUserID int64, snapshot json.RawMessage) (*V1Board, error)
}

// Compile-time convergence: both stores serve the Y seam.
var (
	_ BoardYUpdateStore = (*InMemoryWorkspaceBoardStore)(nil)
	_ BoardYUpdateStore = (*PostgresWorkspaceBoardStore)(nil)
)

func asJSONObject(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, false
	}
	return object, true
}

func asNonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

func asFiniteNumber(value any) (float64, bool) {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

// StripYNode applies the stripTransient whitelist to one Y node entry keyed
// by id. It reports false (drop the entry) when the entry is not an object,
// its id does not equal the map key, or its type is missing.
func StripYNode(key string, entry any) (map[string]any, bool) {
	object, ok := asJSONObject(entry)
	if !ok {
		return nil, false
	}
	id, ok := asNonEmptyString(object["id"])
	if !ok || id != key {
		return nil, false
	}
	nodeType, ok := asNonEmptyString(object["type"])
	if !ok {
		return nil, false
	}
	out := map[string]any{"id": id, "type": nodeType}
	if raw, present := object["position"]; present {
		if position, ok := asJSONObject(raw); ok {
			x, xOK := asFiniteNumber(position["x"])
			y, yOK := asFiniteNumber(position["y"])
			if xOK && yOK {
				out["position"] = map[string]any{"x": x, "y": y}
			}
		}
	}
	if raw, present := object["data"]; present {
		if data, ok := asJSONObject(raw); ok {
			out["data"] = data
		}
	}
	if raw, present := object["dimensions"]; present {
		if dimensions, ok := asJSONObject(raw); ok {
			out["dimensions"] = dimensions
		}
	}
	if raw, present := object["measured"]; present {
		if measured, ok := asJSONObject(raw); ok {
			out["measured"] = measured
		}
	}
	if raw, present := object["style"]; present {
		if style, ok := asJSONObject(raw); ok {
			out["style"] = style
		}
	}
	return out, true
}

// StripYEdge applies the stripTransient whitelist to one Y edge entry keyed
// by id. It reports false when the entry is not an object, its id does not
// equal the map key, or either endpoint is missing.
func StripYEdge(key string, entry any) (map[string]any, bool) {
	object, ok := asJSONObject(entry)
	if !ok {
		return nil, false
	}
	id, ok := asNonEmptyString(object["id"])
	if !ok || id != key {
		return nil, false
	}
	source, ok := asNonEmptyString(object["source"])
	if !ok {
		return nil, false
	}
	target, ok := asNonEmptyString(object["target"])
	if !ok {
		return nil, false
	}
	out := map[string]any{"id": id, "source": source, "target": target}
	if raw, present := object["type"]; present {
		if edgeType, ok := asNonEmptyString(raw); ok {
			out["type"] = edgeType
		}
	}
	if raw, present := object["data"]; present {
		if data, ok := asJSONObject(raw); ok {
			out["data"] = data
		}
	}
	if raw, present := object["style"]; present {
		if style, ok := asJSONObject(raw); ok {
			out["style"] = style
		}
	}
	return out, true
}

func stripYMap(raw any, strip func(string, any) (map[string]any, bool)) map[string]any {
	out := map[string]any{}
	entries, ok := asJSONObject(raw)
	if !ok {
		return out
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if stripped, ok := strip(key, entries[key]); ok {
			out[key] = stripped
		}
	}
	return out
}

// SanitizeYCompactSnapshot enforces the compaction validation boundary: the
// raw snapshot must satisfy the same ValidateV1DocumentStrict rules as the
// non-collaborator path, then its payload is reduced to the sanitized
// nodes/edges maps (stripTransient whitelist, illegal fields and mismatched
// entries dropped). It returns the sanitized envelope ready to persist.
func SanitizeYCompactSnapshot(raw []byte) (int32, json.RawMessage, error) {
	schemaVersion, payload, err := ValidateV1DocumentStrict(raw)
	if err != nil {
		return 0, nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded == nil {
		return 0, nil, ErrInvalidInput
	}
	nodes := stripYMap(decoded[YDocNodesKey], StripYNode)
	edges := stripYMap(decoded[YDocEdgesKey], StripYEdge)
	// Preserve opaque CRDT structs and tombstones with the projection. A JSON
	// projection alone cannot replay updates that reference compacted clocks.
	state, ok := decoded["y_state"].(string)
	stateBytes, stateErr := base64.StdEncoding.DecodeString(state)
	if !ok || stateErr != nil || len(stateBytes) == 0 || len(stateBytes) > v1MaxDocumentBytes {
		return 0, nil, ErrInvalidInput
	}
	if len(nodes)+len(edges) > MaxYCompactEntries {
		return 0, nil, ErrInvalidInput
	}
	projection := map[string]any{"imageNodes": []any{}, "promptNodes": []any{}, "fusionNodes": []any{}, "groups": []any{}, "connections": []any{}, "mediaRefs": []any{}, "y_state": state}
	fields := map[string]string{"image": "imageNodes", "prompt": "promptNodes", "fusion": "fusionNodes", "group": "groups"}
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		node, _ := asJSONObject(nodes[key])
		kind, _ := node["type"].(string)
		field := fields[kind]
		data, ok := asJSONObject(node["data"])
		if !ok || field == "" {
			continue
		}
		if position, ok := asJSONObject(node["position"]); ok {
			data["x"] = position["x"]
			data["y"] = position["y"]
		}
		projection[field] = append(projection[field].([]any), data)
	}
	keys = keys[:0]
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		edge, _ := asJSONObject(edges[key])
		if data, ok := asJSONObject(edge["data"]); ok {
			projection["connections"] = append(projection["connections"].([]any), data)
		}
	}
	schemaVersion = 4
	sanitizedPayload, err := json.Marshal(projection)
	if err != nil {
		return 0, nil, ErrInvalidInput
	}
	envelope, err := json.Marshal(map[string]any{"schema_version": schemaVersion, "payload": json.RawMessage(sanitizedPayload)})
	if err != nil {
		return 0, nil, ErrInvalidInput
	}
	if _, _, err := ValidateV1DocumentStrict(envelope); err != nil {
		return 0, nil, err
	}
	return schemaVersion, json.RawMessage(envelope), nil
}

func validateYUpdateBytes(update []byte) error {
	if len(update) == 0 || len(update) > MaxYUpdateBytes {
		return ErrInvalidInput
	}
	return nil
}

// AppendYUpdate persists one opaque Y update for a live board the author can
// write. It returns the pending (uncompacted) update count for the board.
func (s *InMemoryWorkspaceBoardStore) AppendYUpdate(ctx context.Context, workspacePublicID, boardPublicID string, authorUserID int64, update []byte) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if authorUserID <= 0 {
		return 0, ErrInvalidInput
	}
	if err := validateYUpdateBytes(update); err != nil {
		return 0, err
	}
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (int64, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
		if err != nil {
			return 0, err
		}
		role, visible := s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, authorUserID, roleFor(authorUserID))
		if err := CheckCapability(role, visible, CapWrite); err != nil {
			return 0, err
		}
		if board.ArchivedAt != nil {
			return 0, ErrV1Archived
		}
		stored := make([]byte, len(update))
		copy(stored, update)
		s.nextYUpdateID++
		key := v1BoardKey(workspacePublicID, board.PublicID)
		s.yupdates[key] = append(s.yupdates[key], &YUpdateRecord{
			ID:           s.nextYUpdateID,
			AuthorUserID: authorUserID,
			Update:       stored,
			CreatedAt:    s.clock(),
		})
		return int64(len(s.yupdates[key])), nil
	})
}

// ListYUpdates returns the persisted updates oldest-first (copies).
func (s *InMemoryWorkspaceBoardStore) ListYUpdates(ctx context.Context, workspacePublicID, boardPublicID string, limit int32) ([]*YUpdateRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = MaxYUpdatesListLimit
	}
	if limit > MaxYUpdatesListLimit {
		limit = MaxYUpdatesListLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID)); err != nil {
		return nil, err
	}
	stored := s.yupdates[v1BoardKey(workspacePublicID, strings.TrimSpace(boardPublicID))]
	out := make([]*YUpdateRecord, 0, min(int(limit), len(stored)))
	for _, record := range stored[:min(int(limit), len(stored))] {
		copied := *record
		update := make([]byte, len(record.Update))
		copy(update, record.Update)
		copied.Update = update
		out = append(out, &copied)
	}
	return out, nil
}

// CompactYDocument lands a sanitized Y snapshot: same document accept,
// content identity, projection publish, media-ref reconciliation, and
// immutable revision append as ReplaceDocument, but without an expected
// revision (collaborator convergence, never 409). The board update log is
// trimmed in the same step.
func (s *InMemoryWorkspaceBoardStore) CompactYDocument(ctx context.Context, workspacePublicID, boardPublicID string, authorUserID int64, snapshot json.RawMessage) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if authorUserID <= 0 || len(snapshot) == 0 {
		return nil, ErrInvalidInput
	}
	schemaVersion, sanitized, err := SanitizeYCompactSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	return s.acceptDocument(ctx, workspacePublicID, boardPublicID, authorUserID, nil, schemaVersion, sanitized)
}

// AppendYUpdate persists one opaque Y update for a live board the author can
// write, in the same workspace-locked pattern as the other board mutations.
// It returns the pending (uncompacted) update count for the board.
func (s *PostgresWorkspaceBoardStore) AppendYUpdate(ctx context.Context, workspacePublicID, boardPublicID string, authorUserID int64, update []byte) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if authorUserID <= 0 {
		return 0, ErrInvalidInput
	}
	if err := validateYUpdateBytes(update); err != nil {
		return 0, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var pending int64
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		board, err := lockWritableBoardTx(ctx, q, workspacePublicID, trimmedID, authorUserID)
		if err != nil {
			return err
		}
		if board.ArchivedAt != nil {
			return ErrV1Archived
		}
		stored := make([]byte, len(update))
		copy(stored, update)
		if _, err := q.InsertBoardYUpdate(ctx, dbsqlc.InsertBoardYUpdateParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			AuthorUserID:      authorUserID,
			UpdateData:        stored,
		}); err != nil {
			return err
		}
		pending, err = q.CountBoardYUpdates(ctx, dbsqlc.CountBoardYUpdatesParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	return pending, nil
}

// ListYUpdates returns the persisted updates oldest-first.
func (s *PostgresWorkspaceBoardStore) ListYUpdates(ctx context.Context, workspacePublicID, boardPublicID string, limit int32) ([]*YUpdateRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = MaxYUpdatesListLimit
	}
	if limit > MaxYUpdatesListLimit {
		limit = MaxYUpdatesListLimit
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListBoardYUpdates(ctx, dbsqlc.ListBoardYUpdatesParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		RowLimit:          limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*YUpdateRecord, 0, len(rows))
	for _, row := range rows {
		update := make([]byte, len(row.UpdateData))
		copy(update, row.UpdateData)
		out = append(out, &YUpdateRecord{
			ID:           row.InternalID,
			AuthorUserID: row.AuthorUserID,
			Update:       update,
			CreatedAt:    dbutil.TimeValue(row.CreatedAt),
		})
	}
	return out, nil
}

// CompactYDocument lands a sanitized Y snapshot in the same accepting
// transaction shape as ReplaceDocument (document + content identity +
// projection + media refs + immutable revision, then update-log trim), but
// without an expected revision: concurrent compactions serialize on the board
// row lock and converge last-writer-wins, never 409.
func (s *PostgresWorkspaceBoardStore) CompactYDocument(ctx context.Context, workspacePublicID, boardPublicID string, authorUserID int64, snapshot json.RawMessage) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if authorUserID <= 0 || len(snapshot) == 0 {
		return nil, ErrInvalidInput
	}
	schemaVersion, sanitized, err := SanitizeYCompactSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	return s.acceptDocument(ctx, workspacePublicID, boardPublicID, authorUserID, nil, schemaVersion, sanitized)
}

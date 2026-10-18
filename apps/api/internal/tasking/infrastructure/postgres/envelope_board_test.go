package postgres

import (
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/workspacetasks"
)

func TestEnvelopeRoundTripsBoardBinding(t *testing.T) {
	task := &workspacetasks.WorkspaceTask{
		ID: "task_board_env_1", WorkspaceID: "ws_abc", OwnerUserID: 7,
		Service: "render", MediaAssetIDs: []string{"asset_input_1"},
		Params:  map[string]any{"mask_asset_id": "asset_mask_1"},
		BoardID: "board_canvas_1", BoardAccessVersion: 9,
	}
	raw, err := marshalEnvelope(task)
	if err != nil {
		t.Fatalf("marshalEnvelope: %v", err)
	}
	loaded, err := mapTaskRow(task.WorkspaceID, dbsqlc.Task{WorkflowInput: raw}, nil)
	if err != nil {
		t.Fatalf("mapTaskRow: %v", err)
	}
	if loaded.BoardID != "board_canvas_1" {
		t.Fatalf("board_id = %q, want the bound Board", loaded.BoardID)
	}
	if loaded.BoardAccessVersion != 9 {
		t.Fatalf("board_access_version = %d, want the submit-time snapshot", loaded.BoardAccessVersion)
	}
	if len(loaded.MediaAssetIDs) != 1 || loaded.MediaAssetIDs[0] != "asset_input_1" {
		t.Fatalf("media_asset_ids = %v, want the accepted inputs", loaded.MediaAssetIDs)
	}
	if maskID, ok := workspacetasks.MaskAssetIDFromParams(loaded.Params); !ok || maskID != "asset_mask_1" {
		t.Fatalf("mask = (%q, %v), want the accepted mask", maskID, ok)
	}
}

func TestEnvelopeWithoutBoardReadsBoardLess(t *testing.T) {
	raw := []byte(`{"media_asset_ids":["asset_input_1"],"params":{"output_resolution":"1k"}}`)
	envelope, err := parseEnvelope(raw)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if envelope.BoardID != "" || envelope.BoardAccessVersion != 0 {
		t.Fatalf("unbound envelope = %+v, want Board-less (submitter-only)", envelope)
	}
}

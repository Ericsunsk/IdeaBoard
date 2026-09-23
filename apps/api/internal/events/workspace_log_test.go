package events

import (
	"testing"
	"time"
)

func mustMintV1(t *testing.T, typ, workspace, resourceType, resourceID string, data any) V1Envelope {
	t.Helper()
	envelope, err := MintV1Envelope(typ, workspace, V1Actor{Type: V1ActorUser, ID: "usr_testuser"},
		V1Resource{Type: resourceType, ID: resourceID}, data)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestWorkspaceLogReplayResumesGap(t *testing.T) {
	log := NewWorkspaceEventLog()
	first := mustMintV1(t, EventV1TaskCreated, "ws_1", V1ResourceTask, "task_1",
		V1TaskCreatedData{TaskID: "task_1", WorkflowKind: "render"})
	second := mustMintV1(t, EventV1TaskCompleted, "ws_1", V1ResourceTask, "task_1",
		V1TaskCompletedData{TaskID: "task_1", OutputsURI: "/api/v1/workspaces/ws_1/tasks/task_1/outputs"})
	log.Append(first)
	log.Append(second)

	missed, gap, found := log.ReplayAfter("ws_1", first.ID, WorkspaceEventFilter{})
	if gap || !found {
		t.Fatalf("gap=%v found=%v, want gap=false found=true", gap, found)
	}
	if len(missed) != 1 || missed[0].ID != second.ID {
		t.Fatalf("missed=%v, want exactly the second envelope", missed)
	}
}

func TestWorkspaceLogUnknownCursorResumesFromOldestWithGap(t *testing.T) {
	log := NewWorkspaceEventLog()
	first := mustMintV1(t, EventV1MediaAssetReady, "ws_1", V1ResourceMediaAsset, "asset_1",
		V1MediaAssetReadyData{AssetID: "asset_1"})
	log.Append(first)

	missed, gap, found := log.ReplayAfter("ws_1", "evt_doesnotexist", WorkspaceEventFilter{})
	if !gap || found {
		t.Fatalf("gap=%v found=%v, want gap=true found=false", gap, found)
	}
	if len(missed) != 1 || missed[0].ID != first.ID {
		t.Fatalf("unknown cursor must resume from oldest retained, got %v", missed)
	}
}

func TestWorkspaceLogEmptyCursorIsLiveOnly(t *testing.T) {
	log := NewWorkspaceEventLog()
	log.Append(mustMintV1(t, EventV1TaskCreated, "ws_1", V1ResourceTask, "task_1",
		V1TaskCreatedData{TaskID: "task_1", WorkflowKind: "render"}))
	missed, gap, found := log.ReplayAfter("ws_1", "", WorkspaceEventFilter{})
	if len(missed) != 0 || gap || !found {
		t.Fatalf("empty cursor must be live-only: missed=%d gap=%v found=%v", len(missed), gap, found)
	}
}

func TestWorkspaceLogRetentionBoundaryEvictsOldest(t *testing.T) {
	log := NewWorkspaceEventLogWithLimits(2, 8)
	first := mustMintV1(t, EventV1TaskCreated, "ws_1", V1ResourceTask, "task_1",
		V1TaskCreatedData{TaskID: "task_1", WorkflowKind: "render"})
	second := mustMintV1(t, EventV1TaskRunning, "ws_1", V1ResourceTask, "task_1",
		V1TaskRunningData{TaskID: "task_1"})
	third := mustMintV1(t, EventV1TaskCompleted, "ws_1", V1ResourceTask, "task_1",
		V1TaskCompletedData{TaskID: "task_1", OutputsURI: "/api/v1/workspaces/ws_1/tasks/task_1/outputs"})
	log.Append(first)
	log.Append(second)
	log.Append(third)
	if log.Len("ws_1") != 2 {
		t.Fatalf("retained=%d, want 2 (oldest-first eviction)", log.Len("ws_1"))
	}
	missed, gap, found := log.ReplayAfter("ws_1", first.ID, WorkspaceEventFilter{})
	if !gap || found {
		t.Fatalf("evicted cursor must report gap: gap=%v found=%v", gap, found)
	}
	if len(missed) != 2 {
		t.Fatalf("evicted cursor resumes from oldest retained: got %d", len(missed))
	}
}

func TestWorkspaceLogAppendDedupsOnID(t *testing.T) {
	log := NewWorkspaceEventLog()
	first := mustMintV1(t, EventV1TaskCreated, "ws_1", V1ResourceTask, "task_1",
		V1TaskCreatedData{TaskID: "task_1", WorkflowKind: "render"})
	log.Append(first)
	log.Append(first)
	if log.Len("ws_1") != 1 {
		t.Fatalf("duplicate append must collapse, retained=%d", log.Len("ws_1"))
	}
}

func TestWorkspaceLogFilterNarrowsReplayAndLive(t *testing.T) {
	log := NewWorkspaceEventLog()
	created := mustMintV1(t, EventV1TaskCreated, "ws_1", V1ResourceTask, "task_1",
		V1TaskCreatedData{TaskID: "task_1", WorkflowKind: "render"})
	ready := mustMintV1(t, EventV1MediaAssetReady, "ws_1", V1ResourceMediaAsset, "asset_1",
		V1MediaAssetReadyData{AssetID: "asset_1"})
	log.Append(created)
	log.Append(ready)

	filter, err := NewWorkspaceEventFilter([]string{EventV1TaskCreated, EventV1TaskCompleted})
	if err != nil {
		t.Fatal(err)
	}
	missed, _, _ := log.ReplayAfter("ws_1", "evt_unknown", filter)
	if len(missed) != 1 || missed[0].Type != EventV1TaskCreated {
		t.Fatalf("filtered replay=%v, want only task.created", missed)
	}
	if _, err := NewWorkspaceEventFilter([]string{"task.exploded"}); err == nil {
		t.Fatal("unknown filter type must fail closed")
	}

	stream, unsubscribe, ok := log.Subscribe("ws_1", filter)
	if !ok {
		t.Fatal("subscribe must be accepted")
	}
	defer unsubscribe()
	live := mustMintV1(t, EventV1TaskCompleted, "ws_1", V1ResourceTask, "task_1",
		V1TaskCompletedData{TaskID: "task_1", OutputsURI: "/api/v1/workspaces/ws_1/tasks/task_1/outputs"})
	dropped := mustMintV1(t, EventV1MediaAssetArchived, "ws_1", V1ResourceMediaAsset, "asset_1",
		V1MediaAssetArchivedData{AssetID: "asset_1"})
	log.Append(live)
	log.Append(dropped)
	select {
	case got := <-stream:
		if got.ID != live.ID {
			t.Fatalf("live=%s, want %s (filtered)", got.ID, live.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live envelope")
	}
	select {
	case got := <-stream:
		t.Fatalf("filtered-out envelope must not arrive: %s", got.ID)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWorkspaceLogIsolatesWorkspaces(t *testing.T) {
	log := NewWorkspaceEventLog()
	log.Append(mustMintV1(t, EventV1TaskCreated, "ws_1", V1ResourceTask, "task_1",
		V1TaskCreatedData{TaskID: "task_1", WorkflowKind: "render"}))
	missed, _, _ := log.ReplayAfter("ws_2", "evt_unknown", WorkspaceEventFilter{})
	if len(missed) != 0 {
		t.Fatalf("cross-workspace replay must be empty, got %d", len(missed))
	}
}

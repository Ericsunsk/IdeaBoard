package httpapi

import (
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
)

func ownedEventFixture(t *testing.T) (*v1EventsFixture, *workspacemedia.Asset, *workspacetasks.WorkspaceTask) {
	t.Helper()
	fx := newV1EventsFixture(t)
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	asset := mediaEventUpload(t, media, fx.ws.PublicID, 3, fx.boardID)
	task, err := fx.tasks.CreateTask(t.Context(), &workspacetasks.WorkspaceTask{
		ID: "task_owned_events", WorkspaceID: fx.ws.PublicID, OwnerUserID: 3,
		Service: "render", Status: workspacetasks.StatusRunning, BoardID: fx.boardID,
	})
	if err != nil {
		t.Fatal(err)
	}
	fx.engine = newV1MediaTestEngine(fx.store, fx.dir, ContractV1Deps{Media: media, Boards: fx.boards, Events: fx.eventLog, Tasks: fx.tasks})
	return fx, asset, task
}

func appendOwnedContentEvents(t *testing.T, fx *v1EventsFixture, asset *workspacemedia.Asset, task *workspacetasks.WorkspaceTask) []events.V1Envelope {
	t.Helper()
	var envelopes []events.V1Envelope
	for _, item := range []struct {
		typ      string
		resource events.V1Resource
		data     any
	}{
		{events.EventV1TaskRunning, events.V1Resource{Type: events.V1ResourceTask, ID: task.ID}, events.V1TaskRunningData{TaskID: task.ID}},
		{events.EventV1MediaAssetReady, events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID}, events.V1MediaAssetReadyData{AssetID: asset.ID}},
	} {
		envelope, err := events.MintV1Envelope(item.typ, fx.ws.PublicID, events.V1Actor{Type: "system", ID: "test"}, item.resource, item.data)
		if err != nil {
			t.Fatal(err)
		}
		fx.eventLog.Append(envelope)
		envelopes = append(envelopes, envelope)
	}
	return envelopes
}

func TestLiveSSEIdentityLookupFailureDeniesOwnedContent(t *testing.T) {
	fx, asset, task := ownedEventFixture(t)
	valid, cancelValid, doneValid := openVisibilityStream(t, fx, fx.eventsPath(), 3)
	defer closeVisibilityStream(cancelValid, doneValid)
	waitStreamContains(t, valid, cancelValid, ":connected")
	fx.dir.failPublicIDLookup.Store(true)
	failed, cancelFailed, doneFailed := openVisibilityStream(t, fx, fx.eventsPath(), 3)
	defer closeVisibilityStream(cancelFailed, doneFailed)
	waitStreamContains(t, failed, cancelFailed, ":connected")
	envelopes := appendOwnedContentEvents(t, fx, asset, task)
	for _, envelope := range envelopes {
		waitStreamContains(t, valid, cancelValid, envelope.ID)
	}
	time.Sleep(100 * time.Millisecond)
	for _, envelope := range envelopes {
		if strings.Contains(failed.snapshot(), envelope.ID) {
			t.Fatalf("unresolved identity received live %s", envelope.Type)
		}
	}
}

func TestLiveSSEMembershipRemovalDeniesOwnedContentAndPreservesDirectedLoss(t *testing.T) {
	fx, asset, task := ownedEventFixture(t)
	if _, err := fx.boards.ReplaceBoardAccess(t.Context(), fx.ws.PublicID, fx.boardID, 1, 1, boards.WorkspaceAccessViewer, []int64{3}); err != nil {
		t.Fatal(err)
	}
	stream, cancel, done := openVisibilityStream(t, fx, fx.eventsPath(), 3)
	defer closeVisibilityStream(cancel, done)
	waitStreamContains(t, stream, cancel, ":connected")
	for _, envelope := range appendOwnedContentEvents(t, fx, asset, task) {
		waitStreamContains(t, stream, cancel, envelope.ID)
	}
	if _, err := fx.store.RemoveWorkspaceMember(t.Context(), fx.ws.PublicID, 1, 3, fx.boards); err != nil {
		t.Fatal(err)
	}
	denied := appendOwnedContentEvents(t, fx, asset, task)
	access, err := fx.boards.GetBoardAccess(t.Context(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	record := events.NewBoardAccessOutboxRecord(fx.ws.PublicID, fx.boardID, tenancy.FormatUserPublicID(fx.dir.ensure(3)), access.AccessVersion, time.Now())
	signal, err := record.AccessChangedEnvelope()
	if err != nil {
		t.Fatal(err)
	}
	fx.eventLog.Append(signal)
	snapshot := waitStreamContains(t, stream, cancel, signal.ID)
	for _, envelope := range denied {
		if strings.Contains(snapshot, envelope.ID) {
			t.Fatalf("departed member received owned %s", envelope.Type)
		}
	}
}

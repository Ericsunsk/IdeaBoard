package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tenancy"
)

func mediaEventUpload(t *testing.T, media *workspacemedia.InMemoryWorkspaceMediaStore, workspaceID string, owner int64, boardID string) *workspacemedia.Asset {
	t.Helper()
	session, err := media.CreateSession(t.Context(), workspacemedia.CreateSessionParams{
		WorkspaceID: workspaceID, OwnerUserID: owner, TargetBoardPublicID: boardID,
		Filename: "event.png", MIMEType: "image/png", MediaKind: workspacemedia.MediaKindImage,
		SizeBytes: 64, ChecksumSHA256: strings.Repeat("cd", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, asset, err := media.CompleteSession(t.Context(), workspaceID, session.ID, workspacemedia.CompleteFacts{
		SizeBytes: 64, ChecksumSHA256: strings.Repeat("cd", 32), MediaKind: workspacemedia.MediaKindImage, Width: 100, Height: 80,
	}, workspacemedia.CreateAssetParams{})
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func TestMediaEventsRequireCurrentAssetReadability(t *testing.T) {
	fx := newV1EventsFixture(t)
	ctx := t.Context()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	asset := mediaEventUpload(t, media, fx.ws.PublicID, 1, fx.boardID)
	h := &contractV1Handler{deps: ContractV1Deps{Store: fx.store, Media: media, Boards: fx.boards}}
	owner := workspaceEventViewer{userID: 1, workspaceRole: tenancy.RoleOwner}
	member := workspaceEventViewer{userID: 3, workspaceRole: tenancy.RoleMember}
	for i, typ := range []string{events.EventV1MediaAssetReady, events.EventV1MediaAssetArchived} {
		data, err := json.Marshal(map[string]string{"asset_id": asset.ID})
		if err != nil {
			t.Fatal(err)
		}
		envelope := events.V1Envelope{Type: typ, WorkspaceID: fx.ws.PublicID, Resource: events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID}, Data: data}
		if !h.eventVisibleToViewer(ctx, fx.ws.PublicID, owner, envelope) {
			t.Fatal("asset owner must observe Media events")
		}
		broken := envelope
		broken.Data = json.RawMessage(`{"asset_id":"asset_other"}`)
		if h.eventVisibleToViewer(ctx, fx.ws.PublicID, owner, broken) {
			t.Fatal("mismatched Media event identity was delivered")
		}
		broken.Data = json.RawMessage(`{`)
		if h.eventVisibleToViewer(ctx, fx.ws.PublicID, owner, broken) {
			t.Fatal("malformed Media event was delivered")
		}
		if h.eventVisibleToViewer(ctx, fx.ws.PublicID, member, envelope) {
			t.Fatal("unselected member observed private asset identity")
		}
		version := int64(1 + 2*i)
		if _, err := fx.boards.ReplaceBoardAccess(ctx, fx.ws.PublicID, fx.boardID, 1, version, boards.WorkspaceAccessViewer, []int64{3}); err != nil {
			t.Fatal(err)
		}
		if !h.eventVisibleToViewer(ctx, fx.ws.PublicID, member, envelope) {
			t.Fatal("selected Board reader must observe its Media events")
		}
		if _, err := fx.boards.ReplaceBoardAccess(ctx, fx.ws.PublicID, fx.boardID, 1, version+1, boards.WorkspaceAccessNone, nil); err != nil {
			t.Fatal(err)
		}
		if h.eventVisibleToViewer(ctx, fx.ws.PublicID, member, envelope) {
			t.Fatal("revoked Media event remained visible on replay")
		}
	}
}

func TestContentEventsFailClosedWithoutAuthorizers(t *testing.T) {
	fx := newV1EventsFixture(t)
	h := &contractV1Handler{deps: ContractV1Deps{Store: fx.store}}
	viewer := workspaceEventViewer{userID: 1, workspaceRole: tenancy.RoleOwner}
	for _, envelope := range []events.V1Envelope{
		{Type: events.EventV1TaskRunning, Resource: events.V1Resource{Type: events.V1ResourceTask, ID: "task_unknown"}},
		{Type: events.EventV1MediaAssetReady, Resource: events.V1Resource{Type: events.V1ResourceMediaAsset, ID: "asset_unknown"}, Data: json.RawMessage(`{"asset_id":"asset_unknown"}`)},
	} {
		if h.eventVisibleToViewer(t.Context(), fx.ws.PublicID, viewer, envelope) {
			t.Fatalf("unconfigured authorizer delivered %s", envelope.Type)
		}
	}
}

func TestMediaSSELiveAndReplayDoNotLeakPrivateAssets(t *testing.T) {
	fx := newV1EventsFixture(t)
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	private := mediaEventUpload(t, media, fx.ws.PublicID, 1, fx.boardID)
	sentinel := mediaEventUpload(t, media, fx.ws.PublicID, 3, "")
	fx.engine = newV1MediaTestEngine(fx.store, fx.dir, ContractV1Deps{Media: media, Boards: fx.boards, Events: fx.eventLog, Tasks: fx.tasks})
	member, cancel, done := openVisibilityStream(t, fx, fx.eventsPath(), 3)
	defer closeVisibilityStream(cancel, done)
	waitStreamContains(t, member, cancel, ":connected")
	owner, cancelOwner, doneOwner := openVisibilityStream(t, fx, fx.eventsPath(), 1)
	defer closeVisibilityStream(cancelOwner, doneOwner)
	waitStreamContains(t, owner, cancelOwner, ":connected")
	for _, asset := range []*workspacemedia.Asset{private, sentinel} {
		envelope, err := events.MintV1Envelope(events.EventV1MediaAssetReady, fx.ws.PublicID, events.V1Actor{Type: "system", ID: "test"},
			events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID}, events.V1MediaAssetReadyData{AssetID: asset.ID})
		if err != nil {
			t.Fatal(err)
		}
		fx.eventLog.Append(envelope)
	}
	waitStreamContains(t, owner, cancelOwner, private.ID)
	if snapshot := waitStreamContains(t, member, cancel, sentinel.ID); strings.Contains(snapshot, private.ID) {
		t.Fatal("private Media identity leaked through live SSE")
	}
	if _, err := fx.boards.ReplaceBoardAccess(t.Context(), fx.ws.PublicID, fx.boardID, 1, 1, boards.WorkspaceAccessViewer, []int64{3}); err != nil {
		t.Fatal(err)
	}
	replay, cancelReplay, doneReplay := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_unknown", 3)
	defer closeVisibilityStream(cancelReplay, doneReplay)
	waitStreamContains(t, replay, cancelReplay, private.ID)
	if _, err := fx.boards.ReplaceBoardAccess(t.Context(), fx.ws.PublicID, fx.boardID, 1, 2, boards.WorkspaceAccessNone, nil); err != nil {
		t.Fatal(err)
	}
	revoked, cancelRevoked, doneRevoked := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_unknown", 3)
	defer closeVisibilityStream(cancelRevoked, doneRevoked)
	if snapshot := waitStreamContains(t, revoked, cancelRevoked, sentinel.ID); strings.Contains(snapshot, private.ID) {
		t.Fatal("private Media identity leaked through revoked SSE replay")
	}
}

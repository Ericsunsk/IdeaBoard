package v1

// Board visibility over realtime (issue #940 acceptance).
//
// Servers filter every user-visible event channel by CURRENT permission at
// DELIVERY time; workspaces are never broadcast-then-frontend-filtered.
// These tests prove, through the real SSE transport plus the serve-time
// filter unit, the full acceptance matrix:
//
//   - live + replay filtered by current board access (readers observe
//     board frames, non-readers observe task/billing only);
//   - revoked viewers observe ONLY their directed board.access_changed
//     signal (identity + version, never content), and ordinary replay
//     never returns now-invisible history payloads;
//   - the send-vs-revoke race and the pre-revocation backlog collapse at
//     delivery time (emit-time permission is irrelevant);
//   - old/foreign cursors keep gap semantics with visibility applied;
//   - cross-instance fanout appends converge and serve filtered;
//   - user/workspace switching isolates streams;
//   - authorized task progress links do not regress (refs only, no
//     prompts, no amounts);
//   - webhooks drop restricted board content at dispatch AND at send
//     time, and never carry directed signals.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	eventwebhook "ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// openVisibilityStream attaches a real SSE connection for one user. The
// handler runs until cancel; waitStreamContains polls frames.
func openVisibilityStream(t *testing.T, fx *v1EventsFixture, path string, userID int64) (*testSSEWriter, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	testsupport.SetTestUser(req, userID)
	req.Header.Set(contract.RequestIDHeader, fmt.Sprintf("test-vis-%d-%d", userID, time.Now().UnixNano()))
	writer := newTestSSEWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fx.engine.ServeHTTP(writer, req)
	}()
	return writer, cancel, done
}

func waitStreamContains(t *testing.T, writer *testSSEWriter, cancel context.CancelFunc, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot := writer.snapshot(); strings.Contains(snapshot, want) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatalf("SSE stream never carried %q; got:\n%s", want, writer.snapshot())
	return ""
}

func closeVisibilityStream(cancel context.CancelFunc, done chan struct{}) {
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// replaceBoardDocumentForVisibilityTest replaces the fixture board
// document as the creator and returns the emitted envelope id.
func replaceBoardDocumentForVisibilityTest(t *testing.T, fx *v1EventsFixture, key, ifMatch string) string {
	t.Helper()
	docPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/boards/" + fx.boardID + "/document"
	status, payload := eventIDEM(t, fx.engine, http.MethodPut, docPath, 1, key,
		`{"data":{"schema_version":1,"payload":{"a":1}}}`,
		map[string]string{contract.IfMatchHeader: ifMatch})
	if status != http.StatusOK {
		t.Fatalf("replace document = %d %v, want 200", status, payload)
	}
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	for i := len(missed) - 1; i >= 0; i-- {
		if missed[i].Type == events.EventV1BoardDocumentReplaced {
			return missed[i].ID
		}
	}
	t.Fatal("no document_replaced envelope in the log")
	return ""
}

func addBoardViewerForVisibilityTest(t *testing.T, fx *v1EventsFixture, userID int64, key string) string {
	t.Helper()
	fx.store.SeedWorkspaceMember(fx.ws.PublicID, userID, tenancy.RoleMember)
	public := tenancy.FormatUserPublicID(fx.dir.ensure(userID))
	access, err := fx.boards.GetBoardAccess(t.Context(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]string, 0, len(access.MemberUserIDs)+1)
	for _, id := range access.MemberUserIDs {
		members = append(members, tenancy.FormatUserPublicID(fx.dir.ensure(id)))
	}
	members = append(members, public)
	body, err := json.Marshal(map[string]any{"data": map[string]any{"workspace_access": "viewer", "member_user_ids": members}})
	if err != nil {
		t.Fatal(err)
	}
	status, payload := eventIDEM(t, fx.engine, http.MethodPut, "/api/v1/workspaces/"+fx.ws.PublicID+"/boards/"+fx.boardID+"/access", 1, key, string(body), map[string]string{contract.IfMatchHeader: boards.FormatAccessETag(access.AccessVersion)})
	if status != 200 {
		t.Fatalf("select member=%d %v", status, payload)
	}
	return public
}

func removeBoardMemberForVisibilityTest(t *testing.T, fx *v1EventsFixture, memberPublic, key string) {
	t.Helper()
	access, err := fx.boards.GetBoardAccess(t.Context(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]string, 0, len(access.MemberUserIDs))
	for _, id := range access.MemberUserIDs {
		public := tenancy.FormatUserPublicID(fx.dir.ensure(id))
		if public != memberPublic {
			members = append(members, public)
		}
	}
	body, err := json.Marshal(map[string]any{"data": map[string]any{"workspace_access": "viewer", "member_user_ids": members}})
	if err != nil {
		t.Fatal(err)
	}
	status, payload := eventIDEM(t, fx.engine, http.MethodPut, "/api/v1/workspaces/"+fx.ws.PublicID+"/boards/"+fx.boardID+"/access", 1, key, string(body), map[string]string{contract.IfMatchHeader: boards.FormatAccessETag(access.AccessVersion)})
	if status != 200 {
		t.Fatalf("unselect member=%d %v", status, payload)
	}
}

// accessChangedFor extracts the directed signal for one recipient from a
// raw SSE snapshot and asserts it carries identity + version only.
func accessChangedFor(t *testing.T, snapshot, boardID, memberPublic string) (string, int64) {
	t.Helper()
	var foundID string
	var foundVersion int64
	for _, frame := range strings.Split(snapshot, "\n\n") {
		if !strings.Contains(frame, `"type":"board.access_changed"`) {
			continue
		}
		var dataLine string
		var idLine string
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			}
			if strings.HasPrefix(line, "id: ") {
				idLine = strings.TrimPrefix(line, "id: ")
			}
		}
		var envelope events.V1Envelope
		if err := json.Unmarshal([]byte(dataLine), &envelope); err != nil {
			t.Fatalf("access_changed frame is not an envelope: %v", err)
		}
		var data map[string]any
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data["member_id"] != memberPublic {
			continue
		}
		if len(data) != 3 {
			t.Fatalf("access_changed data keys=%v, want identity+version only (no content)", data)
		}
		if data["board_id"] != boardID {
			t.Fatalf("access_changed board=%v, want %s", data["board_id"], boardID)
		}
		version, ok := data["access_version"].(float64)
		if !ok || version < 1 {
			t.Fatalf("access_changed version=%v, want positive", data["access_version"])
		}
		foundID, foundVersion = idLine, int64(version)
	}
	if foundID == "" {
		t.Fatalf("no directed access_changed for %s in snapshot:\n%s", memberPublic, snapshot)
	}
	// Multiple directed signals converge by version (add-time nudge,
	// then the revocation): the returned frame is the newest observed.
	return foundID, foundVersion
}

func TestBoardSSELiveAndReplayRespectCurrentBoardAccess(t *testing.T) {
	fx := newV1EventsFixture(t)
	// Task first, then the board frame: replay from the task cursor must
	// carry the board frame for readers and drop it for non-readers.
	_, created := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-vis-task", v1CreateTaskBodyJSON("render"), nil)
	if _, ok := created["data"].(map[string]any)["id"].(string); !ok {
		t.Fatalf("task create = %v, want a task id", created)
	}
	docID := replaceBoardDocumentForVisibilityTest(t, fx, "evt-vis-doc", "1")
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	var taskCursor string
	for _, envelope := range missed {
		if envelope.Type == events.EventV1TaskCreated {
			taskCursor = envelope.ID
		}
	}
	if taskCursor == "" {
		t.Fatal("expected a task.created envelope in the log")
	}

	// Replay through an unknown cursor: the non-reader resumes task
	// progress but never board content; the reader resumes both.
	nonReader, cancelNonReader, doneNonReader := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000", 3)
	defer closeVisibilityStream(cancelNonReader, doneNonReader)
	reader, cancelReader, doneReader := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000", 1)
	defer closeVisibilityStream(cancelReader, doneReader)
	waitStreamContains(t, nonReader, cancelNonReader, `"type":"task.created"`)
	waitStreamContains(t, nonReader, cancelNonReader, `"type":"billing.settled"`)
	waitStreamContains(t, reader, cancelReader, docID)
	time.Sleep(300 * time.Millisecond)
	if snapshot := nonReader.snapshot(); strings.Contains(snapshot, docID) {
		t.Fatalf("non-reader replay leaked the board frame:\n%s", snapshot)
	}

	// Live: a newer board frame reaches the reader only.
	docID2 := replaceBoardDocumentForVisibilityTest(t, fx, "evt-vis-doc2", "2")
	waitStreamContains(t, reader, cancelReader, docID2)
	time.Sleep(300 * time.Millisecond)
	if snapshot := nonReader.snapshot(); strings.Contains(snapshot, docID2) {
		t.Fatalf("non-reader live stream leaked the board frame:\n%s", snapshot)
	}

	// Replay from the task cursor: readers resume the board frames,
	// non-readers resume task/billing only.
	replayReader, cancelReplayReader, doneReplayReader := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id="+taskCursor, 1)
	defer closeVisibilityStream(cancelReplayReader, doneReplayReader)
	waitStreamContains(t, replayReader, cancelReplayReader, docID)
	replayNonReader, cancelReplayNonReader, doneReplayNonReader := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id="+taskCursor, 3)
	defer closeVisibilityStream(cancelReplayNonReader, doneReplayNonReader)
	waitStreamContains(t, replayNonReader, cancelReplayNonReader, `"type":"billing.settled"`)
	time.Sleep(300 * time.Millisecond)
	if snapshot := replayNonReader.snapshot(); strings.Contains(snapshot, docID) || strings.Contains(snapshot, docID2) {
		t.Fatalf("non-reader replay leaked a board frame:\n%s", snapshot)
	}
}

func TestBoardSSERevokedViewerReceivesOnlyDirectedSignal(t *testing.T) {
	fx := newV1EventsFixture(t)
	memberPublic := addBoardViewerForVisibilityTest(t, fx, 4, "evt-vis-add4")
	// The grant exists before the stream opens, so the add's
	// member_changed arrives through replay, not a racy live edge.
	stream, cancel, done := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000", 4)
	defer closeVisibilityStream(cancel, done)
	waitStreamContains(t, stream, cancel, `"type":"board.access_changed"`)

	removeBoardMemberForVisibilityTest(t, fx, memberPublic, "evt-vis-rm4")
	docID := replaceBoardDocumentForVisibilityTest(t, fx, "evt-vis-doc2", "1")
	waitStreamContains(t, stream, cancel, `"type":"board.access_changed"`)
	time.Sleep(300 * time.Millisecond)
	snapshot := stream.snapshot()
	if strings.Contains(snapshot, docID) {
		t.Fatalf("revoked viewer received post-revocation board content:\n%s", snapshot)
	}
	signalID, version := accessChangedFor(t, snapshot, fx.boardID, memberPublic)
	if want := events.StableEventID(fx.boardID, memberPublic, strconv.FormatInt(version, 10)); signalID != want {
		t.Fatalf("signal id=%q, want deterministic %q", signalID, want)
	}
	// Ordinary replay with a pre-revocation cursor returns no board
	// payload for the revoked viewer: only their directed signal.
	replay, cancelReplay, doneReplay := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000", 4)
	defer closeVisibilityStream(cancelReplay, doneReplay)
	waitStreamContains(t, replay, cancelReplay, `"type":"board.access_changed"`)
	time.Sleep(200 * time.Millisecond)
	replaySnapshot := replay.snapshot()
	if strings.Contains(replaySnapshot, docID) {
		t.Fatalf("revoked replay returned now-invisible history payload:\n%s", replaySnapshot)
	}
	if !strings.Contains(replaySnapshot, "event: gap") {
		t.Fatalf("unknown cursor must still emit a gap notice:\n%s", replaySnapshot)
	}
}

func TestBoardSSEDeliveryFilterEvaluatesCurrentAccess(t *testing.T) {
	fx := newV1EventsFixture(t)
	memberPublic := addBoardViewerForVisibilityTest(t, fx, 4, "evt-visf-add4")
	docID := replaceBoardDocumentForVisibilityTest(t, fx, "evt-visf-doc", "1")
	removeBoardMemberForVisibilityTest(t, fx, memberPublic, "evt-visf-rm4")

	h := &contractV1Handler{deps: ContractV1Deps{Store: fx.store, Boards: fx.boards, Users: fx.dir}}
	newPrincipal := func(userID int64, role string) principal.Principal {
		p, err := principal.New(userID, fx.dir.ensure(userID),
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	ctx := context.Background()
	viewer4, ok := h.eventViewerFor(ctx, newPrincipal(4, "user"), tenancy.RoleMember)
	if !ok {
		t.Fatal("viewer identity must resolve")
	}
	viewer1, ok := h.eventViewerFor(ctx, newPrincipal(1, "user"), tenancy.RoleOwner)
	if !ok {
		t.Fatal("owner identity must resolve")
	}
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	var doc, signal events.V1Envelope
	for _, envelope := range missed {
		switch {
		case envelope.ID == docID:
			doc = envelope
		case events.IsDirectedAccessSignal(envelope):
			signal = envelope
		}
	}
	if doc.ID == "" || signal.ID == "" {
		t.Fatal("expected the board frame and the directed signal in the log")
	}
	// The frame was emitted while user 4 was authorized; delivery after
	// the revocation drops it (send-vs-revoke race + backlog collapse).
	if h.eventVisibleToViewer(ctx, fx.ws.PublicID, viewer4, doc) {
		t.Fatal("pre-revocation backlog must not deliver after revocation")
	}
	if !h.eventVisibleToViewer(ctx, fx.ws.PublicID, viewer1, doc) {
		t.Fatal("reader must still observe the board frame")
	}
	if !h.eventVisibleToViewer(ctx, fx.ws.PublicID, viewer4, signal) {
		t.Fatal("revoked viewer must observe their directed signal")
	}
	if h.eventVisibleToViewer(ctx, fx.ws.PublicID, viewer1, signal) {
		t.Fatal("directed signal must not leak to other readers")
	}
	other, ok := h.eventViewerFor(ctx, newPrincipal(3, "user"), tenancy.RoleMember)
	if !ok {
		t.Fatal("member identity must resolve")
	}
	if h.eventVisibleToViewer(ctx, fx.ws.PublicID, other, signal) {
		t.Fatal("directed signal must not leak to uninvolved members")
	}
	// Malformed board scope fails closed.
	broken := doc
	broken.Data = []byte(`{"revision":2}`)
	if h.eventVisibleToViewer(ctx, fx.ws.PublicID, viewer1, broken) {
		t.Fatal("board frame without board identity must fail closed")
	}
}

func TestBoardSSECrossInstanceBridgeServesFiltered(t *testing.T) {
	fx := newV1EventsFixture(t)
	memberPublic := addBoardViewerForVisibilityTest(t, fx, 4, "evt-bridge-add4")
	docID := replaceBoardDocumentForVisibilityTest(t, fx, "evt-bridge-doc", "1")

	// Instance B learns instance A's commit through the NATS bridge
	// (decode stage already resulted in delivery; the handler appends).
	logB := events.NewWorkspaceEventLog()
	bridge := events.NewWorkspaceEventBridgeHandler(logB)
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	for _, envelope := range missed {
		if err := bridge(context.Background(), envelope); err != nil {
			t.Fatalf("bridge append: %v", err)
		}
	}
	if logB.Len(fx.ws.PublicID) != len(missed) {
		t.Fatalf("instance B log=%d, want %d bridged envelopes", logB.Len(fx.ws.PublicID), len(missed))
	}
	h := &contractV1Handler{deps: ContractV1Deps{Store: fx.store, Boards: fx.boards, Users: fx.dir}}
	viewer := func(userID int64, role, workspaceRole string) workspaceEventViewer {
		t.Helper()
		p, err := principal.New(userID, fx.dir.ensure(userID),
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil {
			t.Fatal(err)
		}
		v, ok := h.eventViewerFor(context.Background(), p, workspaceRole)
		if !ok {
			t.Fatal("viewer identity must resolve")
		}
		return v
	}
	remote, _, _ := logB.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	nonReader := viewer(3, "user", tenancy.RoleMember)
	granted := viewer(4, "user", tenancy.RoleMember)
	owner := viewer(1, "user", tenancy.RoleOwner)
	_ = memberPublic
	for _, envelope := range remote {
		if envelope.ID == docID {
			if h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, nonReader, envelope) {
				t.Fatal("bridged board frame must stay invisible to non-readers")
			}
			if !h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, granted, envelope) {
				t.Fatal("bridged board frame must reach the granted viewer")
			}
			if !h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, owner, envelope) {
				t.Fatal("bridged board frame must reach the owner")
			}
		}
		if events.IsDirectedAccessSignal(envelope) {
			recipient, _ := events.AccessChangedRecipient(envelope)
			if h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, granted, envelope) != (recipient == memberPublic) {
				t.Fatal("bridged directed signal must reach its recipient")
			}
			if h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, nonReader, envelope) {
				t.Fatal("bridged directed signal must not leak")
			}
		}
	}
}

func TestBoardSSEWorkspaceIsolation(t *testing.T) {
	fx := newV1EventsFixture(t)
	ctx := context.Background()
	ws2, err := fx.store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "events-2", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	fx.store.SeedWorkspaceMember(ws2.PublicID, 3, tenancy.RoleMember)
	_, created := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+ws2.PublicID+"/tasks", 3, "evt-iso-task", v1CreateTaskBodyJSON("render"), nil)
	otherTaskID, _ := created["data"].(map[string]any)["id"].(string)
	if otherTaskID == "" {
		t.Fatalf("ws2 task create = %v", created)
	}
	// A ws1 stream never observes ws2 frames.
	stream, cancel, done := openVisibilityStream(t, fx, fx.eventsPath(), 3)
	defer closeVisibilityStream(cancel, done)
	waitStreamContains(t, stream, cancel, ":connected")
	time.Sleep(300 * time.Millisecond)
	if snapshot := stream.snapshot(); strings.Contains(snapshot, otherTaskID) {
		t.Fatalf("ws1 stream leaked a ws2 task frame:\n%s", snapshot)
	}
	// A ws1 cursor cannot resume on ws2: unknown there, so gap + refetch.
	stream2, cancel2, done2 := openVisibilityStream(t, fx,
		"/api/v1/workspaces/"+ws2.PublicID+"/events?last_event_id=evt_doesnotexist0000", 3)
	defer closeVisibilityStream(cancel2, done2)
	snapshot := waitStreamContains(t, stream2, cancel2, "event: gap")
	if !strings.Contains(snapshot, otherTaskID) {
		t.Fatalf("ws2 replay must carry its own task frame:\n%s", snapshot)
	}
}

func TestTaskProgressFramesCarryRefsOnly(t *testing.T) {
	fx := newV1EventsFixture(t)
	_, created := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-refs-task", v1CreateTaskBodyJSON("render"), nil)
	taskID, _ := created["data"].(map[string]any)["id"].(string)
	if taskID == "" {
		t.Fatalf("task create = %v", created)
	}
	// Emit-before-open with an unknown cursor: replay is deterministic
	// (no subscribe-vs-emit race), then assert the payload shape.
	stream, cancel, done := openVisibilityStream(t, fx, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000", 3)
	defer closeVisibilityStream(cancel, done)
	snapshot := waitStreamContains(t, stream, cancel, `"type":"task.created"`)
	waitStreamContains(t, stream, cancel, `"type":"billing.settled"`)
	for _, frame := range strings.Split(snapshot, "\n\n") {
		if !strings.Contains(frame, `"type":"task.created"`) {
			continue
		}
		if strings.Contains(frame, `"prompt"`) {
			t.Fatalf("task progress must not carry prompts:\n%s", frame)
		}
		var line string
		for _, text := range strings.Split(frame, "\n") {
			if strings.HasPrefix(text, "data: ") {
				line = strings.TrimPrefix(text, "data: ")
			}
		}
		var envelope events.V1Envelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatal(err)
		}
		for key := range data {
			switch key {
			case "task_id", "service":
			default:
				t.Fatalf("task.created data key=%q, want refs only", key)
			}
		}
	}
}

func TestBoardAccessUpdateIdempotentReplayEmitsOnce(t *testing.T) {
	fx := newV1EventsFixture(t)
	addBoardViewerForVisibilityTest(t, fx, 4, "evt-idem-add4")
	accessPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/boards/" + fx.boardID + "/access"
	status, _ := eventIDEM(t, fx.engine, http.MethodPut, accessPath, 1, "evt-idem-access",
		`{"data":{"workspace_access":"editor","member_user_ids":[]}}`, map[string]string{contract.IfMatchHeader: "access-2"})
	if status != http.StatusOK {
		t.Fatalf("access update = %d, want 200", status)
	}
	before := len(fx.logTypes())
	// Same key + same body replays the first result without re-emitting
	// directed signals (deterministic ids would
	// converge anyway, but silence is the contract).
	status, _ = eventIDEM(t, fx.engine, http.MethodPut, accessPath, 1, "evt-idem-access",
		`{"data":{"workspace_access":"editor","member_user_ids":[]}}`, map[string]string{contract.IfMatchHeader: "access-2"})
	if status != http.StatusOK {
		t.Fatalf("access replay = %d, want 200", status)
	}
	if got := len(fx.logTypes()); got != before {
		t.Fatalf("idempotent replay emitted %d envelopes, want still %d", got-before, before)
	}
}

type captureEventEnqueuer struct {
	mu   sync.Mutex
	args []eventwebhook.WebhookDeliveryArgs
}

func (m *captureEventEnqueuer) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	delivery, ok := args.(eventwebhook.WebhookDeliveryArgs)
	if !ok {
		return nil, fmt.Errorf("capture enqueuer: unexpected args %T", args)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.args = append(m.args, delivery)
	return &rivertype.JobInsertResult{}, nil
}

func (m *captureEventEnqueuer) captured() []eventwebhook.WebhookDeliveryArgs {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]eventwebhook.WebhookDeliveryArgs(nil), m.args...)
}

func TestWebhookBoardContentNeverReachesWorkspaceSubscriptions(t *testing.T) {
	fx := newV1EventsFixture(t)
	status, payload := eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-boardvis",
		`{"data":{"url":"https://hooks.example.com/board"}}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("create subscription = %d %v, want 201", status, payload)
	}
	capturer := &captureEventEnqueuer{}
	fx.webhooks.SetEnqueuer(capturer)
	memberPublic := addBoardViewerForVisibilityTest(t, fx, 4, "evt-wh-add4")
	replaceBoardDocumentForVisibilityTest(t, fx, "evt-wh-selected-doc", "1")
	removeBoardMemberForVisibilityTest(t, fx, memberPublic, "evt-wh-rm4")
	replaceBoardDocumentForVisibilityTest(t, fx, "evt-wh-private-doc", "2")
	if got := len(capturer.captured()); got != 0 {
		t.Fatalf("Board content or directed signals enqueued %d Workspace webhooks", got)
	}

	// The worker enforces the same audience boundary for durable jobs.
	envelope, err := events.MintV1Envelope(events.EventV1BoardDocumentReplaced, fx.ws.PublicID,
		events.V1Actor{Type: events.V1ActorSystem, ID: "system"},
		events.V1Resource{Type: events.V1ResourceBoard, ID: fx.boardID},
		events.V1BoardDocumentReplacedData{BoardID: fx.boardID, Revision: 3})
	if err != nil {
		t.Fatal(err)
	}
	subscriptions := fx.webhooks.List(fx.ws.PublicID)
	if len(subscriptions) != 1 {
		t.Fatalf("subscriptions=%d, want 1", len(subscriptions))
	}
	worker := eventwebhook.NewWebhookDeliverWorker(fx.webhooks)
	for attempt := 1; attempt <= 2; attempt++ {
		err := worker.Work(context.Background(), &river.Job[eventwebhook.WebhookDeliveryArgs]{
			JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 8},
			Args: eventwebhook.WebhookDeliveryArgs{SubscriptionID: subscriptions[0].ID,
				DeliveryID: envelope.ID, Envelope: envelope},
		})
		if err != nil {
			t.Fatalf("Board delivery must drop terminally, got %v", err)
		}
	}
	if calls, _, _ := fx.sender.snapshot(); calls != 0 {
		t.Fatalf("Board content sent %d Workspace webhooks", calls)
	}
}

func TestWorkspaceMemberReaddPreservesBoardGrants(t *testing.T) {
	fx := newV1EventsFixture(t)
	memberPublic := addBoardViewerForVisibilityTest(t, fx, 4, "workspace-repeat-grant")
	ctx := context.Background()
	before, err := fx.boards.GetBoardAccess(ctx, fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	for _, key := range []string{"workspace-repeat-first", "workspace-repeat-second"} {
		status, payload := eventIDEM(t, fx.engine, http.MethodPost,
			"/api/v1/workspaces/"+fx.ws.PublicID+"/members", 1, key,
			fmt.Sprintf(`{"data":{"user_id":%q,"role":"admin"}}`, memberPublic), nil)
		if status != http.StatusForbidden {
			t.Fatalf("direct Personal re-add = %d %v, want 403", status, payload)
		}
		if role, ok, err := fx.store.GetWorkspaceRole(ctx, fx.ws.PublicID, 4); err != nil || !ok || role != tenancy.RoleMember {
			t.Fatalf("workspace role = %q, %t, %v", role, ok, err)
		}
		if role, ok, err := fx.boards.GetBoardRole(ctx, fx.ws.PublicID, fx.boardID, 4, tenancy.RoleMember); err != nil || !ok || role != "viewer" {
			t.Fatalf("board role = %q, %t, %v", role, ok, err)
		}
		after, err := fx.boards.GetBoardAccess(ctx, fx.ws.PublicID, fx.boardID)
		if err != nil || after.AccessVersion != before.AccessVersion {
			t.Fatalf("access changed = %+v, %v", after, err)
		}
	}
	eventsAfter, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("re-add emitted events: before %d, after %d", len(eventsBefore), len(eventsAfter))
	}
}

func TestWorkspaceMemberRemovePurgesBoardGrants(t *testing.T) {
	fx := newV1EventsFixture(t)
	memberPublic := addBoardViewerForVisibilityTest(t, fx, 4, "evt-purge-add4")
	access, err := fx.boards.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatalf("get access version: %v", err)
	}
	versionBefore := access.AccessVersion
	scanSignals := func() (directed []int64) {
		missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
		for _, envelope := range missed {
			switch envelope.Type {
			case events.EventV1BoardAccessChanged:
				recipient, ok := events.AccessChangedRecipient(envelope)
				if !ok || recipient != memberPublic {
					continue
				}
				version, ok := events.AccessChangedVersion(envelope)
				if !ok {
					t.Fatal("directed signal must carry its version")
				}
				directed = append(directed, version)
			}
		}
		return directed
	}

	// Workspace membership removal purges the departed member's board
	// grants in the same idempotent op (#941): the roster row is gone,
	// the live board bumps exactly once, and both convergence signals
	// carry the post-bump version.
	memberPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/members/" + memberPublic
	status, payload := eventIDEM(t, fx.engine, http.MethodDelete, memberPath, 1, "evt-purge-rm4", "", nil)
	if status != http.StatusOK {
		t.Fatalf("member remove = %d %v, want 200", status, payload)
	}
	grants, err := fx.boards.ListBoardMembers(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	for _, grant := range grants {
		if grant != nil && grant.UserID == 4 {
			t.Fatalf("departed member grant survived the workspace removal: %+v", grants)
		}
	}
	after, err := fx.boards.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatalf("get access after remove: %v", err)
	}
	if after.AccessVersion != versionBefore+1 {
		t.Fatalf("access version = %d, want one bump to %d", after.AccessVersion, versionBefore+1)
	}
	if _, ok, _ := fx.boards.GetBoardRole(context.Background(), fx.ws.PublicID, fx.boardID, 4, tenancy.RoleMember); ok {
		t.Fatal("departed member must lose board visibility")
	}
	directed := scanSignals()
	if len(directed) != 2 || directed[0] != versionBefore || directed[1] != versionBefore+1 {
		t.Fatalf("directed signals to the departed member=%v, want [grant v%d, removal v%d]", directed, versionBefore, versionBefore+1)
	}

	// Same-key replay replays the first result without re-emitting or
	// re-bumping: the purge already converged.
	status, _ = eventIDEM(t, fx.engine, http.MethodDelete, memberPath, 1, "evt-purge-rm4", "", nil)
	if status != http.StatusOK {
		t.Fatalf("member remove replay = %d, want 200", status)
	}
	if again, err := fx.boards.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID); err != nil || again.AccessVersion != versionBefore+1 {
		t.Fatalf("replay moved version to %+v, %v; want still %d", again, err, versionBefore+1)
	}

	// Rejoining requires an accepted invitation and does not restore board grants.
	inv, _, err := fx.store.CreateWorkspaceMemberInvite(context.Background(), fx.ws.PublicID, 1, "rejoin@example.com", tenancy.RoleMember, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.store.AcceptWorkspaceInvite(context.Background(), inv.PublicID, 4, "rejoin@example.com"); err != nil {
		t.Fatal(err)
	}

	if _, ok, _ := fx.boards.GetBoardRole(context.Background(), fx.ws.PublicID, fx.boardID, 4, tenancy.RoleMember); ok {
		t.Fatal("rejoin must not resurrect the purged board grant")
	}
	directed = scanSignals()
	if len(directed) != 2 {
		t.Fatalf("directed signals after rejoin=%v, want still the grant + removal pair", directed)
	}
}

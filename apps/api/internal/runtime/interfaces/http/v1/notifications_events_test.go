package v1

// Notification realtime hints (P1): backend emission + per-user stream.
//
// Covers the post-commit hint contract only: frozen envelope validity,
// deterministic ids (replay-safe), workspace-log fanout, recipient user-hub
// fanout, per-user SSE delivery (frames, gap, filters, fail-closed drops),
// workspace SSE visibility (member in / outsider out), and the webhook
// exclusion. Organization/billing notifications stay poll-only in this cut
// (no ws_ home for a v1 envelope).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	eventwebhook "ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type stubInviteIdentities struct {
	emails map[int64]string
}

func (s *stubInviteIdentities) Email(_ context.Context, userID int64) (string, error) {
	if email, ok := s.emails[userID]; ok && email != "" {
		return email, nil
	}
	return "", fmt.Errorf("stub identities: unknown user %d", userID)
}

func (s *stubInviteIdentities) FindUser(_ context.Context, email string) (int64, error) {
	for id, known := range s.emails {
		if strings.EqualFold(known, strings.TrimSpace(email)) {
			return id, nil
		}
	}
	return 0, nil
}

type notificationHintFixture struct {
	engine *gin.Engine
	store  *tenancy.InMemoryStore
	dir    *staticUserDirectory
	notifs *notifications.MemoryStore
	log    *events.WorkspaceEventLog
	hub    *events.UserEventHub
	ws     *tenancy.Workspace
}

func newNotificationHintFixture(t *testing.T) *notificationHintFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	const owner, invitee = 10, 20
	for _, id := range []int64{owner, invitee} {
		dir.ensure(id)
	}
	ws := v1WorkspaceFixture(t, ctx, store, "hint-space", owner)
	mem := notifications.NewMemoryStore()
	identities := &stubInviteIdentities{emails: map[int64]string{
		owner: "owner@example.com", invitee: "invitee@example.com",
	}}
	eventLog := events.NewWorkspaceEventLog()
	userHub := events.NewUserEventHub()
	commands := notifications.NewService(mem, &notifications.EventPublisher{Log: eventLog, Hub: userHub})
	dispatcher := tenancyevents.NewCollaborationEventDispatcher(commands, nil)
	engine := newV1MediaTestEngine(store, dir, ContractV1Deps{
		Notifications: commands,
		Invitations:   testInvitations(store, nil, dispatcher, identities),
		InvitationReader: testInvitationReader(dir, &staticMemberDisplayProvider{
			emails: map[int64]string{owner: "owner@example.com", invitee: "invitee@example.com"},
		}),
		UserEvents: userHub,
	})
	return &notificationHintFixture{
		engine: engine, store: store, dir: dir, notifs: mem,
		log: eventLog, hub: userHub, ws: ws,
	}
}

func (f *notificationHintFixture) createInvite(t *testing.T, key, email string) string {
	t.Helper()
	status, _, payload := doV1(t, f.engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + f.ws.PublicID + "/invites",
		userID: 10,
		body:   fmt.Sprintf(`{"email":%q,"role":"member"}`, email),
		headers: map[string]string{
			contract.IdempotencyKeyHeader: key,
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create invite = %d %v, want 201", status, payload)
	}
	id, _ := v1Data(t, payload).(map[string]any)["id"].(string)
	if !strings.HasPrefix(id, tenancy.WorkspaceInvitePrefix) {
		t.Fatalf("invite id = %q, want wsi_ prefix", id)
	}
	return id
}

func (f *notificationHintFixture) hintTypes() []string {
	missed, _, _ := f.log.ReplayAfter(f.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	types := make([]string, 0, len(missed))
	for _, envelope := range missed {
		types = append(types, envelope.Type)
	}
	return types
}

func requireHintEnvelope(t *testing.T, envelope events.V1Envelope, typ, inviteID, workspaceID string) {
	t.Helper()
	if err := events.ValidateV1Envelope(envelope); err != nil {
		t.Fatalf("hint envelope invalid: %v", err)
	}
	if envelope.Type != typ {
		t.Fatalf("hint type = %q, want %q", envelope.Type, typ)
	}
	if envelope.WorkspaceID != workspaceID {
		t.Fatalf("hint workspace = %q, want %q", envelope.WorkspaceID, workspaceID)
	}
	if envelope.Resource.Type != events.V1ResourceInvite || envelope.Resource.ID != inviteID {
		t.Fatalf("hint resource = %+v, want invite %q", envelope.Resource, inviteID)
	}
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["invite_id"] != inviteID {
		t.Fatalf("hint data invite_id = %v, want %q", data["invite_id"], inviteID)
	}
	notificationType, _ := data["notification_type"].(string)
	if notificationType == "" {
		t.Fatalf("hint data lacks notification_type: %v", data)
	}
	if len(data) != 2 {
		t.Fatalf("hint data keys = %v, want invite_id + notification_type only", data)
	}
}

// subscribeHub taps the recipient hub BEFORE the triggering action: the
// hub is live-only (no replay), so late subscribers miss earlier fanout.
func subscribeHub(t *testing.T, hub *events.UserEventHub, userID int64) (<-chan events.UserEvent, func()) {
	t.Helper()
	stream, unsubscribe, ok := hub.Subscribe(userID)
	if !ok || stream == nil {
		t.Fatalf("hub subscribe user %d refused", userID)
	}
	return stream, unsubscribe
}

func readHubEvent(t *testing.T, stream <-chan events.UserEvent, userID int64, wantType string) events.UserEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-stream:
			if event.UserID != userID {
				t.Fatalf("hub event user = %d, want %d", event.UserID, userID)
			}
			if wantType != "" && event.Type != wantType {
				continue
			}
			return event
		case <-deadline:
			t.Fatalf("hub user %d never received %q", userID, wantType)
			return events.UserEvent{}
		}
	}
}

func requireHubHintEnvelope(t *testing.T, event events.UserEvent, typ, inviteID, workspaceID string) {
	t.Helper()
	var envelope events.V1Envelope
	if err := json.Unmarshal(event.Data, &envelope); err != nil {
		t.Fatalf("hub event data is not a v1 envelope: %v", err)
	}
	requireHintEnvelope(t, envelope, typ, inviteID, workspaceID)
}

func TestNotificationHintCreateInviteEmitsLogAndRecipient(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const invitee = 20
	inviteeStream, unsubscribe := subscribeHub(t, fx.hub, invitee)
	defer unsubscribe()

	inviteID := fx.createInvite(t, "hint-create-1", "invitee@example.com")

	missed, _, _ := fx.log.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	if len(missed) != 1 {
		t.Fatalf("log has %d envelopes, want 1", len(missed))
	}
	requireHintEnvelope(t, missed[0], events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)

	// The invitee holds no membership yet: their hint arrives through the
	// recipient user hub, not the workspace log audience.
	requireHubHintEnvelope(t, readHubEvent(t, inviteeStream, invitee, events.EventV1NotificationCreated),
		events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)

	// Same idempotency key replays without re-emitting.
	status, _, _ := doV1(t, fx.engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + fx.ws.PublicID + "/invites",
		userID: 10,
		body:   `{"email":"invitee@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "hint-create-1",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("replay = %d, want 201", status)
	}
	if got := len(fx.hintTypes()); got != 1 {
		t.Fatalf("replay emitted %d envelopes, want still 1", got)
	}
}

func TestNotificationHintInviteAcceptEmitsUpdatedAndCreated(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const owner, invitee = 10, 20
	inviteeStream, unsubscribeInvitee := subscribeHub(t, fx.hub, invitee)
	defer unsubscribeInvitee()
	ownerStream, unsubscribeOwner := subscribeHub(t, fx.hub, owner)
	defer unsubscribeOwner()

	inviteID := fx.createInvite(t, "hint-accept-1", "invitee@example.com")
	// Consume the create-time fanout so the decision assertions below read
	// only decision fanout.
	readHubEvent(t, inviteeStream, invitee, events.EventV1NotificationCreated)
	before := len(fx.hintTypes())

	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/user/invites/" + inviteID + "/accept",
		userID: invitee,
	})
	if status != http.StatusOK {
		t.Fatalf("accept = %d %v, want 200", status, payload)
	}
	after := fx.hintTypes()[before:]
	if len(after) != 2 || after[0] != events.EventV1NotificationUpdated || after[1] != events.EventV1NotificationCreated {
		t.Fatalf("accept emitted %v, want [updated created]", after)
	}
	missed, _, _ := fx.log.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	decision := missed[before:]
	requireHintEnvelope(t, decision[0], events.EventV1NotificationUpdated, inviteID, fx.ws.PublicID)
	requireHintEnvelope(t, decision[1], events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)
	var createdData map[string]any
	if err := json.Unmarshal(decision[1].Data, &createdData); err != nil {
		t.Fatal(err)
	}
	if createdData["notification_type"] != string(notifications.TypeWorkspaceMemberJoined) {
		t.Fatalf("created notification_type = %v, want member-joined", createdData["notification_type"])
	}

	// Invitee (self) observes the resolution; inviter observes the notice.
	requireHubHintEnvelope(t, readHubEvent(t, inviteeStream, invitee, events.EventV1NotificationUpdated),
		events.EventV1NotificationUpdated, inviteID, fx.ws.PublicID)
	requireHubHintEnvelope(t, readHubEvent(t, ownerStream, owner, events.EventV1NotificationCreated),
		events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)
}

func TestNotificationHintInviteDeclineEmitsDeclinedNotice(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const owner, invitee = 10, 20
	ownerStream, unsubscribeOwner := subscribeHub(t, fx.hub, owner)
	defer unsubscribeOwner()

	inviteID := fx.createInvite(t, "hint-decline-1", "invitee@example.com")
	before := len(fx.hintTypes())

	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/user/invites/" + inviteID + "/decline",
		userID: invitee,
	})
	if status != http.StatusOK {
		t.Fatalf("decline = %d %v, want 200", status, payload)
	}
	after := fx.hintTypes()[before:]
	if len(after) != 2 || after[0] != events.EventV1NotificationUpdated || after[1] != events.EventV1NotificationCreated {
		t.Fatalf("decline emitted %v, want [updated created]", after)
	}
	missed, _, _ := fx.log.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	var createdData map[string]any
	if err := json.Unmarshal(missed[before+1].Data, &createdData); err != nil {
		t.Fatal(err)
	}
	if createdData["notification_type"] != string(notifications.TypeWorkspaceInviteDeclined) {
		t.Fatalf("decline notice type = %v, want invite-declined", createdData["notification_type"])
	}
	requireHubHintEnvelope(t, readHubEvent(t, ownerStream, owner, events.EventV1NotificationCreated),
		events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)
}

func TestNotificationHintVisibilityFollowsMembership(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const owner, outsider = 10, 99
	fx.dir.ensure(outsider)
	inviteID := fx.createInvite(t, "hint-vis-1", "invitee@example.com")
	missed, _, _ := fx.log.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	if len(missed) == 0 {
		t.Fatal("expected a hint envelope in the log")
	}
	requireHintEnvelope(t, missed[len(missed)-1], events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)

	h := &contractV1Handler{deps: ContractV1Deps{Store: fx.store, Users: fx.dir}}
	newViewer := func(userID int64, workspaceRole string) workspaceEventViewer {
		t.Helper()
		p, err := principal.New(userID, fx.dir.ensure(userID),
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), "user")
		if err != nil {
			t.Fatal(err)
		}
		viewer, ok := h.eventViewerFor(context.Background(), p, workspaceRole)
		if !ok {
			t.Fatal("viewer identity must resolve")
		}
		return viewer
	}
	member := newViewer(owner, tenancy.RoleOwner)
	if !h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, member, missed[len(missed)-1]) {
		t.Fatal("member must observe the notification hint")
	}
	nonMember := newViewer(outsider, "user")
	if h.eventVisibleToViewer(context.Background(), fx.ws.PublicID, nonMember, missed[len(missed)-1]) {
		t.Fatal("outsider must not observe the notification hint")
	}
}

func TestUserNotificationStreamServesRecipientHints(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const invitee = 20
	stream, cancel, done := openHintStream(t, fx.engine, "/api/v1/user/notifications/events", invitee)
	defer closeHintStream(cancel, done)
	waitHubSubscriber(t, fx.hub, invitee, 1)

	inviteID := fx.createInvite(t, "hint-stream-1", "invitee@example.com")
	snapshot := waitHintStreamContains(t, stream, cancel, `"type":"notification.created"`)
	if !strings.Contains(snapshot, inviteID) {
		t.Fatalf("user stream never carried the invite hint:\n%s", snapshot)
	}
	var frames int
	for _, frame := range strings.Split(snapshot, "\n\n") {
		if !strings.Contains(frame, `"type":"notification.created"`) {
			continue
		}
		frames++
		for _, line := range strings.Split(frame, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var envelope events.V1Envelope
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &envelope); err != nil {
				t.Fatalf("hint frame is not an envelope: %v", err)
			}
			requireHintEnvelope(t, envelope, events.EventV1NotificationCreated, inviteID, fx.ws.PublicID)
		}
	}
	if frames != 1 {
		t.Fatalf("stream carried %d created frames, want 1", frames)
	}
}

func TestUserNotificationStreamGapFilterAndFailClosed(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const invitee = 20
	// A cursor can never be honored without a replay window: the frame must
	// arrive with a gap notice, and the client takes the full pull.
	cursor := events.StableEventID("notification", events.EventV1NotificationCreated, "wsi_probe")
	stream, cancel, done := openHintStream(t, fx.engine,
		"/api/v1/user/notifications/events?last_event_id="+cursor, invitee)
	defer closeHintStream(cancel, done)
	snapshot := waitHintStreamContains(t, stream, cancel, "event: gap")
	if !strings.Contains(snapshot, cursor) {
		t.Fatalf("gap frame must echo the cursor %q:\n%s", cursor, snapshot)
	}

	// Types filter narrows delivery: updated-only never sees created.
	filtered, cancelFiltered, doneFiltered := openHintStream(t, fx.engine,
		"/api/v1/user/notifications/events?types=notification.updated", invitee)
	defer closeHintStream(cancelFiltered, doneFiltered)
	waitHubSubscriber(t, fx.hub, invitee, 2)
	inviteID := fx.createInvite(t, "hint-stream-2", "invitee@example.com")
	waitHintStreamContains(t, stream, cancel, inviteID)
	time.Sleep(300 * time.Millisecond)
	if snapshot := filtered.snapshot(); strings.Contains(snapshot, `"type":"notification.created"`) {
		t.Fatalf("filtered stream leaked a created hint:\n%s", snapshot)
	}

	// Non-notification hub traffic fails closed (dropped, never framed).
	fx.hub.Publish(events.UserEvent{UserID: invitee, Type: events.EventTypeTaskUpdated, Data: json.RawMessage(`{}`)})
	fx.hub.Publish(events.UserEvent{UserID: invitee, Type: "notification.exploded", Data: json.RawMessage(`{}`)})
	time.Sleep(300 * time.Millisecond)
	for _, writer := range []*testSSEWriter{stream, filtered} {
		if snapshot := writer.snapshot(); strings.Contains(snapshot, "task.updated") || strings.Contains(snapshot, "notification.exploded") {
			t.Fatalf("unknown hub types must be dropped:\n%s", snapshot)
		}
	}

	// Unknown types filter fails closed with 400 before any stream opens.
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodGet,
		path:   "/api/v1/user/notifications/events?types=task.exploded",
		userID: invitee,
	})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("bad filter = %d %v, want 422 validation_failed", status, payload)
	}
}

func TestUserNotificationStreamAuthAndAvailability(t *testing.T) {
	fx := newNotificationHintFixture(t)
	// Anonymous fails closed with 401.
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodGet,
		path:   "/api/v1/user/notifications/events",
	})
	if status != http.StatusUnauthorized || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon stream = %d %v, want 401", status, payload)
	}
	// Unwired hints fail closed with 503, never an empty stream.
	plain := newV1MediaTestEngine(fx.store, fx.dir, ContractV1Deps{Notifications: fx.notifs})
	status, _, _ = doV1(t, plain, v1Call{
		method: http.MethodGet,
		path:   "/api/v1/user/notifications/events",
		userID: 20,
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("unwired stream = %d, want 503", status)
	}
}

func TestNotificationHintsNeverRideWebhooks(t *testing.T) {
	if eventwebhook.IsWorkspaceWebhookEventType(events.EventV1NotificationCreated) ||
		eventwebhook.IsWorkspaceWebhookEventType(events.EventV1NotificationUpdated) {
		t.Fatal("notification hints must stay out of the webhook allowlist")
	}
	fx := newNotificationHintFixture(t)
	// Workspace webhook subscriptions reject notification types at create time.
	engine := newV1MediaTestEngine(fx.store, fx.dir, ContractV1Deps{
		Notifications: fx.notifs,
	})
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + fx.ws.PublicID + "/event-subscriptions",
		userID: 10,
		body:   `{"data":{"url":"https://hooks.example.com/hints","event_types":["notification.created"]}}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "hint-webhook-1",
		},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("webhook subscribe with notification.created = %d %v, want 422", status, payload)
	}
}

func waitHubSubscriber(t *testing.T, hub *events.UserEventHub, userID int64, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount(userID) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hub has %d subscribers for %d, want >= %d", hub.SubscriberCount(userID), userID, want)
}

func openHintStream(t *testing.T, engine *gin.Engine, path string, userID int64) (*testSSEWriter, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	testsupport.SetTestUser(req, userID)
	req.Header.Set(contract.RequestIDHeader, fmt.Sprintf("test-hint-%d-%d", userID, time.Now().UnixNano()))
	writer := newTestSSEWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.ServeHTTP(writer, req)
	}()
	return writer, cancel, done
}

func waitHintStreamContains(t *testing.T, writer *testSSEWriter, cancel context.CancelFunc, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot := writer.snapshot(); strings.Contains(snapshot, want) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatalf("user stream never carried %q; got:\n%s", want, writer.snapshot())
	return ""
}

func closeHintStream(cancel context.CancelFunc, done chan struct{}) {
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func TestNotificationReadArchiveEmitsUpdatedHint(t *testing.T) {
	fx := newNotificationHintFixture(t)
	const invitee = 20
	inviteeStream, unsubscribeInvitee := subscribeHub(t, fx.hub, invitee)
	defer unsubscribeInvitee()

	inviteID := fx.createInvite(t, "hint-read-1", "invitee@example.com")
	// Consume the create-time fanout so assertions below read only the
	// read/archive fanout.
	readHubEvent(t, inviteeStream, invitee, events.EventV1NotificationCreated)

	items, _, err := fx.notifs.ListNotifications(context.Background(), invitee, notifications.ListFilter{}, 10, 0)
	if err != nil || len(items) == 0 {
		t.Fatalf("list invitee notifications = %d %v, want >=1", len(items), err)
	}
	notifID := items[0].PublicID
	before := len(fx.hintTypes())

	status, _, _ := doV1(t, fx.engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/user/notifications/" + notifID + "/read",
		userID: invitee,
	})
	if status != http.StatusOK {
		t.Fatalf("mark read = %d, want 200", status)
	}
	after := fx.hintTypes()[before:]
	if len(after) != 1 || after[0] != events.EventV1NotificationUpdated {
		t.Fatalf("read emitted %v, want [updated]", after)
	}
	missed, _, _ := fx.log.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	decision := missed[before:]
	requireHintEnvelope(t, decision[0], events.EventV1NotificationUpdated, inviteID, fx.ws.PublicID)
	requireHubHintEnvelope(t, readHubEvent(t, inviteeStream, invitee, events.EventV1NotificationUpdated),
		events.EventV1NotificationUpdated, inviteID, fx.ws.PublicID)

	// Archive is a distinct persisted state and must remain replayable after read.
	beforeArchive := len(fx.hintTypes())
	status, _, _ = doV1(t, fx.engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/user/notifications/" + notifID + "/archive",
		userID: invitee,
	})
	if status != http.StatusOK {
		t.Fatalf("archive = %d, want 200", status)
	}
	if got := len(fx.hintTypes()); got != beforeArchive+1 {
		t.Fatalf("archive log has %d envelopes, want %d (distinct archive fact)", got, beforeArchive+1)
	}
	requireHubHintEnvelope(t, readHubEvent(t, inviteeStream, invitee, events.EventV1NotificationUpdated),
		events.EventV1NotificationUpdated, inviteID, fx.ws.PublicID)
}

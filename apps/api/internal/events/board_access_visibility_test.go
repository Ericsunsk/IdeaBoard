package events

// Board access-signal visibility (issue #940). Covers the delivery-time
// rules: board content reaches current readers only and directed
// signals reach the named member only, with the payload limited to
// board identity plus the access version.

import (
	"encoding/json"
	"testing"
)

func mustMintBoardAccessChanged(t *testing.T, boardID, memberID string, version int64) V1Envelope {
	t.Helper()
	envelope, err := MintV1Envelope(EventV1BoardAccessChanged, "ws_abc123",
		V1Actor{Type: V1ActorService, ID: "board-access"},
		V1Resource{Type: V1ResourceBoard, ID: boardID},
		V1BoardAccessChangedData{BoardID: boardID, MemberID: memberID, AccessVersion: version})
	if err != nil {
		t.Fatalf("mint access_changed: %v", err)
	}
	return envelope
}

func TestAccessChangedPayloadCarriesIdentityAndVersionOnly(t *testing.T) {
	envelope := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 3 {
		t.Fatalf("access_changed data keys=%v, want exactly board_id+member_id+access_version", data)
	}
	if data["board_id"] != "board_abc123" || data["member_id"] != "usr_member1" || data["access_version"].(float64) != 3 {
		t.Fatalf("access_changed data=%v", data)
	}
}

func TestAccessChangedValidationRejectsSmuggledContent(t *testing.T) {
	base := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	for name, raw := range map[string]string{
		"role":      `{"board_id":"board_abc123","member_id":"usr_member1","access_version":3,"role":"viewer"}`,
		"prompt":    `{"board_id":"board_abc123","member_id":"usr_member1","access_version":3,"prompt":"secret"}`,
		"asset":     `{"board_id":"board_abc123","member_id":"usr_member1","access_version":3,"asset_id":"asset_x"}`,
		"zero":      `{"board_id":"board_abc123","member_id":"usr_member1","access_version":0}`,
		"badmember": `{"board_id":"board_abc123","member_id":"alice","access_version":3}`,
	} {
		candidate := base
		candidate.Data = json.RawMessage(raw)
		if err := ValidateV1Envelope(candidate); err == nil {
			t.Fatalf("access_changed with %s must fail closed", name)
		}
	}
}

func TestBoardScopeHelpers(t *testing.T) {
	doc, err := MintV1Envelope(EventV1BoardDocumentReplaced, "ws_abc123", testActor(),
		V1Resource{Type: V1ResourceBoard, ID: "board_abc123"},
		V1BoardDocumentReplacedData{BoardID: "board_abc123", Revision: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !IsBoardScopedEventType(doc.Type) || IsDirectedAccessSignal(doc) {
		t.Fatal("document_replaced must be board-scoped but not directed")
	}
	if boardID, ok := BoardIDOf(doc); !ok || boardID != "board_abc123" {
		t.Fatalf("BoardIDOf=%q %v", boardID, ok)
	}
	task, err := MintV1Envelope(EventV1TaskCreated, "ws_abc123", testActor(),
		V1Resource{Type: V1ResourceTask, ID: "task_abc123"},
		V1TaskCreatedData{TaskID: "task_abc123", Service: "render"})
	if err != nil {
		t.Fatal(err)
	}
	if IsBoardScopedEventType(task.Type) || IsDirectedAccessSignal(task) {
		t.Fatal("task.created must not be board-scoped")
	}
	if _, ok := BoardIDOf(task); ok {
		t.Fatal("task.created must carry no board scope")
	}
	signal := mustMintBoardAccessChanged(t, "board_abc123", "usr_member1", 3)
	if !IsDirectedAccessSignal(signal) {
		t.Fatal("access_changed must be directed")
	}
	if recipient, ok := AccessChangedRecipient(signal); !ok || recipient != "usr_member1" {
		t.Fatalf("recipient=%q %v", recipient, ok)
	}
	if version, ok := AccessChangedVersion(signal); !ok || version != 3 {
		t.Fatalf("version=%d %v", version, ok)
	}
	if _, ok := AccessChangedRecipient(doc); ok {
		t.Fatal("document_replaced must have no directed recipient")
	}
}

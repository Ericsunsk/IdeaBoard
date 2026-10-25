package invitations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

func TestRiverMailEnqueueSharesInvitationTransaction(t *testing.T) {
	mailEncryption(t)
	f := newFixture(t)
	var schema string
	if err := f.db.QueryRow(t.Context(), "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(f.db), &rivermigrate.Config{Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	producer, err := river.NewClient(riverpgxv5.New(f.db), &river.Config{Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	queue := &MailQueue{Client: producer}
	failure := errors.New("injected notification insert failure")
	f.service.Events = tenancyevents.NewCollaborationEventDispatcher(notificationFailure{Store: f.notifications, createErr: failure}, queue)
	if _, _, err := f.service.CreateWorkspaceMemberInvite(t.Context(), f.workspace.PublicID, f.owner, "member@example.com", tenancy.RoleMember); !errors.Is(err, failure) {
		t.Fatalf("expected rollback: %v", err)
	}
	var jobs int
	if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM river_job").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("job escaped rollback: %d %v", jobs, err)
	}
	f.service.Events = tenancyevents.NewCollaborationEventDispatcher(f.notifications, queue)
	invite, token, err := f.service.CreateWorkspaceMemberInvite(t.Context(), f.workspace.PublicID, f.owner, "member@example.com", tenancy.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	if err := f.db.QueryRow(t.Context(), "SELECT args FROM river_job").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), token) {
		t.Fatal("plaintext token in River persistence")
	}
	var args WorkspaceMailArgs
	if err := json.Unmarshal(payload, &args); err != nil {
		t.Fatal(err)
	}
	// A new worker instance has no in-process token/state; it reconstructs the
	// invitation from PostgreSQL plus the encrypted, durable River arguments.
	sender := &mailSenderSpy{}
	restartedWorker := &MailWorker{Store: f.store, Sender: sender}
	if err := restartedWorker.Work(t.Context(), &river.Job[WorkspaceMailArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 || sender.input.Token != token || sender.input.InviteID != invite.PublicID {
		t.Fatal("restart lost mail delivery")
	}
	if err := f.service.CancelWorkspaceMemberInvite(t.Context(), f.workspace.PublicID, invite.PublicID, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := restartedWorker.Work(t.Context(), &river.Job[WorkspaceMailArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 {
		t.Fatal("cancelled invite retried mail")
	}
	// The queue also refuses an ambient-less enqueue, never falling back to
	// a separate transaction that could survive an invitation rollback.
	if err := queue.SendWorkspaceInviteEmail(context.Background(), invite, token); err == nil {
		t.Fatal("nontransactional enqueue succeeded")
	}
}

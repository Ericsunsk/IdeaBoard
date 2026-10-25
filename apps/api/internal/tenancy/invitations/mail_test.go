package invitations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	authclient "ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type txStub struct{ pgx.Tx }
type mailQueueSpy struct {
	args WorkspaceMailArgs
	tx   pgx.Tx
	opts *river.InsertOpts
	err  error
}

func (s *mailQueueSpy) InsertTx(_ context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	s.args, s.tx, s.opts = args.(WorkspaceMailArgs), tx, opts
	return nil, s.err
}

type mailReaderStub struct {
	invite *tenancy.WorkspaceMemberInvite
	err    error
}

func (s mailReaderStub) GetWorkspaceMemberInvite(context.Context, string) (*tenancy.WorkspaceMemberInvite, error) {
	return s.invite, s.err
}

type mailSenderSpy struct {
	calls int
	input authclient.WorkspaceInvitationMail
	err   error
}

func (s *mailSenderSpy) SendWorkspaceInvitation(_ context.Context, input authclient.WorkspaceInvitationMail) error {
	s.calls++
	s.input = input
	return s.err
}

func mailEncryption(t *testing.T) {
	t.Helper()
	previous := secrets.EncryptionKey
	secrets.EncryptionKey = []byte("01234567890123456789012345678901")
	t.Cleanup(func() { secrets.EncryptionKey = previous })
}
func TestMailQueueRequiresTransactionAndEncryptsToken(t *testing.T) {
	mailEncryption(t)
	token := strings.Repeat("a", 64)
	invite := &tenancy.WorkspaceMemberInvite{
		PublicID: "wsi_test", InviteeEmail: "person@example.com", WorkspaceName: "Team", Role: tenancy.RoleMember,
	}
	spy := &mailQueueSpy{}
	queue := &MailQueue{Client: spy}
	if err := queue.SendWorkspaceInviteEmail(t.Context(), invite, token); err == nil {
		t.Fatal("enqueued without transaction")
	}
	tx := &txStub{}
	ctx := dbutil.ContextWithAmbientTx(t.Context(), tx)
	if err := queue.SendWorkspaceInviteEmail(ctx, invite, token); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(spy.args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), token) {
		t.Fatal("plaintext token in durable job")
	}
	if spy.tx != tx || spy.args.InviteID != invite.PublicID || spy.args.KeyVersion != secrets.KeyVersion() {
		t.Fatal("wrong atomic delivery facts")
	}
	if spy.args.Recipient != invite.InviteeEmail || spy.args.WorkspaceName != invite.WorkspaceName || spy.args.Role != invite.Role || spy.args.InviterName != "Workspace administrator" {
		t.Fatal("missing immutable mail facts")
	}
	if spy.opts.Queue != riverpolicy.RiverQueueWorkspaceInvitationMail || spy.opts.MaxAttempts != riverpolicy.RiverMaxAttemptsWorkspaceInvitationMail || !spy.opts.UniqueOpts.ByArgs {
		t.Fatal("wrong delivery policy")
	}
	decoded, err := secrets.DecryptString(spy.args.TokenCiphertext)
	if err != nil || decoded != token {
		t.Fatalf("token cannot be recovered: %v", err)
	}
	failure := errors.New("queue failed")
	spy.err = failure
	if err := queue.SendWorkspaceInviteEmail(ctx, invite, token); !errors.Is(err, failure) {
		t.Fatalf("lost enqueue error: %v", err)
	}
}

func TestMailWorkerUsesCurrentInviteAndRetrySafeDelivery(t *testing.T) {
	mailEncryption(t)
	token := strings.Repeat("a", 64)
	encrypted, err := secrets.EncryptString(token)
	if err != nil {
		t.Fatal(err)
	}
	invite := &tenancy.WorkspaceMemberInvite{PublicID: "wsi_test", InviteeEmail: "person@example.com", WorkspaceName: "Team", Role: tenancy.RoleMember, Status: tenancy.MemberInviteStatusPending, ExpiresAt: time.Now().Add(time.Hour), TokenHash: tenancy.MemberInviteTokenHash(token)}
	job := &river.Job[WorkspaceMailArgs]{Args: WorkspaceMailArgs{
		InviteID: invite.PublicID, Recipient: invite.InviteeEmail, WorkspaceName: invite.WorkspaceName,
		InviterName: "Workspace administrator", Role: invite.Role,
		TokenCiphertext: encrypted, KeyVersion: secrets.KeyVersion(),
	}}
	sender := &mailSenderSpy{}
	worker := &MailWorker{Store: mailReaderStub{invite: invite}, Sender: sender}
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 || sender.input.Token != token || sender.input.InviteID != invite.PublicID || sender.input.Recipient != invite.InviteeEmail {
		t.Fatal("mail not sent using invitation facts")
	}
	failure := errors.New("auth unavailable")
	sender.err = failure
	if err := worker.Work(t.Context(), job); !errors.Is(err, failure) {
		t.Fatal("delivery failure swallowed")
	}
	for _, status := range []string{tenancy.MemberInviteStatusCancelled, tenancy.MemberInviteStatusAccepted, tenancy.MemberInviteStatusDeclined, tenancy.MemberInviteStatusExpired} {
		invite.Status = status
		if err := worker.Work(t.Context(), job); err != nil {
			t.Fatal(err)
		}
	}
	invite.Status = tenancy.MemberInviteStatusPending
	invite.ExpiresAt = time.Now().Add(-time.Second)
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 2 {
		t.Fatal("resolved/expired invite was sent")
	}
	invite.ExpiresAt = time.Now().Add(time.Hour)
	job.Args.KeyVersion = "old-generation"
	if err := worker.Work(t.Context(), job); err == nil {
		t.Fatal("key rotation did not fail closed")
	}
	job.Args.KeyVersion = secrets.KeyVersion()
	invite.TokenHash = "mismatched"
	if err := worker.Work(t.Context(), job); err == nil {
		t.Fatal("swapped token did not fail closed")
	}
}

func TestMailRetryKeepsQueuedPayloadAfterInvitationFactsChange(t *testing.T) {
	mailEncryption(t)
	token := strings.Repeat("a", 64)
	invite := &tenancy.WorkspaceMemberInvite{
		PublicID: "wsi_test", InviteeEmail: "person@example.com", WorkspaceName: "Original Team",
		Role: tenancy.RoleMember, Status: tenancy.MemberInviteStatusPending,
		ExpiresAt: time.Now().Add(time.Hour), TokenHash: tenancy.MemberInviteTokenHash(token),
	}
	queueSpy := &mailQueueSpy{}
	queue := &MailQueue{Client: queueSpy}
	ctx := dbutil.ContextWithAmbientTx(t.Context(), &txStub{})
	if err := queue.SendWorkspaceInviteEmail(ctx, invite, token); err != nil {
		t.Fatal(err)
	}
	// Reconstruct a durable job without depending on the original in-process
	// invitation. A lost provider response must retry exactly the same payload.
	payload, err := json.Marshal(queueSpy.args)
	if err != nil {
		t.Fatal(err)
	}
	var args WorkspaceMailArgs
	if err := json.Unmarshal(payload, &args); err != nil {
		t.Fatal(err)
	}
	job := &river.Job[WorkspaceMailArgs]{Args: args}
	lostResponse := errors.New("provider response lost")
	sender := &mailSenderSpy{err: lostResponse}
	worker := &MailWorker{Store: mailReaderStub{invite: invite}, Sender: sender}
	if err := worker.Work(t.Context(), job); !errors.Is(err, lostResponse) {
		t.Fatalf("delivery failure not retryable: %v", err)
	}
	first := sender.input
	if first.WorkspaceName != "Original Team" || first.Recipient != "person@example.com" || first.Role != tenancy.RoleMember {
		t.Fatal("first delivery did not use the queued snapshot")
	}
	invite.WorkspaceName = "Renamed Team"
	invite.InviteeEmail = "changed@example.com"
	invite.Role = tenancy.RoleAdmin
	sender.err = nil
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 2 || sender.input != first {
		t.Fatalf("retry payload changed: first=%#v retry=%#v", first, sender.input)
	}
}

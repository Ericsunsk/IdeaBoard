package invitations

import (
	"context"
	"errors"
	"fmt"
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

const WorkspaceMailKind = "workspace_invitation_mail"

// WorkspaceMailArgs keeps token plaintext out of River rows, errors and logs.
// Only InviteID participates in uniqueness, so ciphertext nonces do not cause
// duplicate delivery jobs for the same immutable invitation. Template facts are
// captured at enqueue time so retries use the same provider idempotency payload.
type WorkspaceMailArgs struct {
	InviteID        string `json:"invite_id"        river:"unique"`
	Recipient       string `json:"recipient"`
	WorkspaceName   string `json:"workspace_name"`
	InviterName     string `json:"inviter_name"`
	Role            string `json:"role"`
	TokenCiphertext string `json:"token_ciphertext"`
	KeyVersion      string `json:"key_version"`
}

func (WorkspaceMailArgs) Kind() string { return WorkspaceMailKind }

type MailEnqueuer interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type MailQueue struct{ Client MailEnqueuer }

// SendWorkspaceInviteEmail schedules delivery rather than performing external
// I/O. Requiring the ambient transaction makes enqueue atomic with the invite.
func (q *MailQueue) SendWorkspaceInviteEmail(ctx context.Context, invite *tenancy.WorkspaceMemberInvite, token string) error {
	tx, ok := dbutil.AmbientTxFromContext(ctx)
	if !ok || q == nil || q.Client == nil || invite == nil || token == "" {
		return errors.New("invitations: transactional mail queue unavailable")
	}
	ciphertext, err := secrets.EncryptString(token)
	if err != nil {
		return err
	}
	_, err = q.Client.InsertTx(ctx, tx, WorkspaceMailArgs{
		InviteID: invite.PublicID, Recipient: invite.InviteeEmail,
		WorkspaceName: invite.WorkspaceName, InviterName: "Workspace administrator", Role: invite.Role,
		TokenCiphertext: ciphertext, KeyVersion: secrets.KeyVersion(),
	}, &river.InsertOpts{
		Queue:       riverpolicy.RiverQueueWorkspaceInvitationMail,
		MaxAttempts: riverpolicy.RiverMaxAttemptsWorkspaceInvitationMail,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	})
	return err
}

type MailSender interface {
	SendWorkspaceInvitation(context.Context, authclient.WorkspaceInvitationMail) error
}

type MailInviteReader interface {
	GetWorkspaceMemberInvite(context.Context, string) (*tenancy.WorkspaceMemberInvite, error)
}

// MailWorker checks current invitation eligibility on every attempt, while
// rendering from the immutable queued snapshot. Auth derives the provider
// idempotency key from InviteID, including retries after a worker restart.
type MailWorker struct {
	river.WorkerDefaults[WorkspaceMailArgs]
	Store  MailInviteReader
	Sender MailSender
}

func (w *MailWorker) Timeout(*river.Job[WorkspaceMailArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutWorkspaceInvitationMail
}

func (w *MailWorker) Work(ctx context.Context, job *river.Job[WorkspaceMailArgs]) error {
	if w == nil || w.Store == nil || w.Sender == nil || job == nil {
		return errors.New("invitations: mail worker unavailable")
	}
	invite, err := w.Store.GetWorkspaceMemberInvite(ctx, job.Args.InviteID)
	if errors.Is(err, tenancy.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if invite.Status != tenancy.MemberInviteStatusPending || !time.Now().Before(invite.ExpiresAt) {
		return nil
	}
	if job.Args.KeyVersion == "" || job.Args.KeyVersion != secrets.KeyVersion() {
		return fmt.Errorf("invitations: mail encryption key generation unavailable for %s", job.Args.InviteID)
	}
	token, err := secrets.DecryptString(job.Args.TokenCiphertext)
	if err != nil {
		return errors.New("invitations: cannot decrypt queued mail token")
	}
	if tenancy.MemberInviteTokenHash(token) != invite.TokenHash {
		return errors.New("invitations: queued mail token does not match invitation")
	}
	return w.Sender.SendWorkspaceInvitation(ctx, authclient.WorkspaceInvitationMail{
		Recipient: job.Args.Recipient, InviteID: job.Args.InviteID,
		WorkspaceName: job.Args.WorkspaceName, InviterName: job.Args.InviterName,
		Role: job.Args.Role, Token: token,
	})
}

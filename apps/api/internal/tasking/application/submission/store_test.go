package submission

import (
	"context"
	"errors"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestStoreSubmitTaskRollsBackTaskWhenRiverInsertFails(t *testing.T) {
	ctx := context.Background()
	db, pool := testdb.OpenWithPool(t)

	canonical, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42,
		Prompt: "draw a chair", UserPrompt: "draw a chair",
		Params: `{"size":"1024x1024"}`, Feature: "test-non-billable",
		Priority: tasks.TaskPriorityDefault,
	})
	if err != nil {
		t.Fatalf("NewCanonicalRequest() error = %v", err)
	}
	store := newTestStore(pool, taskstore.NewRepository(db), failingRiverTxClient{err: errors.New("river insert failed")}, "drawing_default", 3, nil)

	if _, err := store.SubmitTask(ctx, canonical); err == nil {
		t.Fatalf("expected submit to fail")
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE user_id = $1`, int64(42)).Scan(&count); err != nil {
		t.Fatalf("count task failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("task row count=%d, want 0", count)
	}
}

func TestStoreRejectsUnpreparedGenerationMediaBeforeTaskInsert(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	canonical, err := NewCanonicalRequest(CanonicalRequestInput{
		UserID: 42, Prompt: "draw", UserPrompt: "draw", Params: `{"schema_version":5}`,
		Feature: "test-non-billable", ClientActionID: "unprepared-media",
		Generation: &tasks.GenerationRequestSnapshot{
			SchemaVersion: tasks.CurrentParamsSchemaVersion, Feature: "test-non-billable", Primitive: "image-to-image",
			TemplateFingerprint: "unprepared-media", RequestedOutputCount: 1,
			Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, Source: "/uploads/source.png"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newTestStore(pool, taskstore.NewRepository(db), failingRiverTxClient{err: errors.New("River must not be called")}, "drawing_default", 3, nil)
	if _, err := store.SubmitTask(ctx, canonical); err == nil {
		t.Fatal("expected unprepared media submission to fail")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE user_id = $1`, int64(42)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("task row count=%d, want 0", count)
	}
}

func TestStoreRejectsMissingBillingWithoutPartialFacts(t *testing.T) {
	ctx := t.Context()
	db, pool := testdb.OpenWithPool(t)
	canonical, constructErr := NewCanonicalRequest(CanonicalRequestInput{
		UserID:         42,
		Feature:        "variations",
		ClientActionID: "missing-billing",
	})
	if constructErr == nil {
		t.Fatal("expected canonical construction to reject missing Billing")
	}

	client := &countingRiverTxClient{inner: failingRiverTxClient{err: errors.New("unexpected River insert")}}
	store := newTestStore(pool, taskstore.NewRepository(db), client, "drawing_default", 3, nil)
	if _, err := store.SubmitTask(ctx, canonical); err == nil {
		t.Fatal("expected zero canonical request to be rejected")
	}

	for name, query := range map[string]string{
		"tasks":     `SELECT COUNT(*) FROM tasks`,
		"snapshots": `SELECT COUNT(*) FROM task_billing_snapshots`,
		"holds":     `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'hold'`,
	} {
		var count int
		if err := db.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s count=%d, want no partial facts", name, count)
		}
	}
	if client.InsertCount() != 0 {
		t.Fatalf("River insert count=%d, want 0", client.InsertCount())
	}
}

type failingRiverTxClient struct {
	err error
}

func (c failingRiverTxClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, c.err
}

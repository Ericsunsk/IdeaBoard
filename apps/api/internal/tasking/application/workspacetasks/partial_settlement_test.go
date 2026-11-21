package workspacetasks

import (
	"fmt"
	"testing"
)

// Real PostgreSQL proves the actual-output Capture and unused Hold Release
// remain atomic when a four-image batch completes with only accepted outputs.
func TestAtomicBatchPartialCapturesActualOutputs(t *testing.T) {
	for accepted := 1; accepted < 4; accepted++ {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			env := openAtomicEnv(t, 10)
			created, err := env.service.Create(t.Context(), CreateInput{
				WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render",
				CanonicalParams: canonicalQuantityParams(t, "render", 4),
			}, "batch-partial")
			if err != nil {
				t.Fatal(err)
			}
			env.claimTask(t, created.ID, created.ExecutionToken)
			outputs := make([]OutputRef, accepted)
			for i := range outputs {
				outputs[i] = OutputRef{Kind: OutputKindRemote, URL: fmt.Sprintf("https://cdn.example.com/partial-%d.png", i), ChecksumSHA256: atomicTestChecksum, ResultIndex: i}
			}
			completed, err := env.service.Complete(t.Context(), env.ws, created.ID, outputs)
			if err != nil {
				t.Fatal(err)
			}
			if completed.Status != StatusCompleted || len(completed.OutputRefs) != accepted || completed.RetryCount != 0 || env.probeCount(t, created.ID) != 1 {
				t.Fatalf("completed=%+v, want accepted outputs without requeue", completed)
			}
			if available, reserved := env.wallet(t); available != int64(1000-10*accepted) || reserved != 0 {
				t.Fatalf("wallet=(%d,%d), want (%d,0)", available, reserved, 1000-10*accepted)
			}
			if env.ledgerCount(t, `idempotency_key = $1`, "task-capture:"+created.ID) != 1 || env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
				t.Fatal("partial completion must capture once and release the unused hold once")
			}
		})
	}
}

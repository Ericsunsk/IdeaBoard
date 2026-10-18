package tests

import (
	"bytes"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/rewards"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	rewardscredits "ideaboard/api/internal/rewards/infrastructure/credits"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
	"strings"
	"sync"
	"testing"
)

func TestRewardCodeCreateReplaySurvivesHMACRotation(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := rewards.CodeKey{Version: "old", Secret: bytes.Repeat([]byte{0x31}, 32)}
	newKey := rewards.CodeKey{Version: "new", Secret: bytes.Repeat([]byte{0x32}, 32)}
	oldKeyring, err := rewards.NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := rewards.NewService(pool, rewardscredits.New(creditService), oldKeyring)
	command := ports.CreateRewardCodeCommand{
		DisplayName: "轮换奖励", Code: "ROTATE-CODE-2026", CreditAmount: 12,
		IdempotencyKey: "reward-code-rotation", ActorUserID: 1,
	}
	created, err := service.CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := rewards.NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	rotatedService := rewards.NewService(pool, rewardscredits.New(creditService), rotatedKeyring)
	replay, err := rotatedService.CreateRewardCode(t.Context(), command)
	if err != nil || replay.RewardCode.ID != created.RewardCode.ID || !replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("rotation replay = %+v error=%v", replay, err)
	}
}

func TestRewardCodeCreateReplaySurvivesReverseHMACRotation(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := rewards.CodeKey{Version: "reverse-old", Secret: bytes.Repeat([]byte{0x33}, 32)}
	newKey := rewards.CodeKey{Version: "reverse-new", Secret: bytes.Repeat([]byte{0x34}, 32)}
	oldKeyring, err := rewards.NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := rewards.NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	command := ports.CreateRewardCodeCommand{
		DisplayName: "反向轮换奖励", Code: "REVERSE-ROTATE-2026", CreditAmount: 14,
		IdempotencyKey: "reward-code-reverse-rotation", ActorUserID: 1,
	}
	created, err := rewards.NewService(pool, rewardscredits.New(creditService), rotatedKeyring).CreateRewardCode(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := rewards.NewService(pool, rewardscredits.New(creditService), oldKeyring).CreateRewardCode(t.Context(), command)
	if err != nil || replay.RewardCode.ID != created.RewardCode.ID || !replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("reverse rotation replay = %+v error=%v", replay, err)
	}
	differentCode := command
	differentCode.Code = "DIFFERENT-ROTATE-2026"
	if _, err := rewards.NewService(pool, rewardscredits.New(creditService), oldKeyring).CreateRewardCode(t.Context(), differentCode); !errors.Is(err, rewardsdomain.ErrRewardCodeConflict) {
		t.Fatalf("same idempotency with different Code error = %v, want conflict", err)
	}
	differentPayload := command
	differentPayload.CreditAmount++
	if _, err := rewards.NewService(pool, rewardscredits.New(creditService), rotatedKeyring).CreateRewardCode(t.Context(), differentPayload); !errors.Is(err, rewardsdomain.ErrRewardCodeConflict) {
		t.Fatalf("same idempotency with different payload error = %v, want conflict", err)
	}
}

func TestRewardCodeCreateConcurrentIdempotencyAcrossRollingKeyrings(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := rewards.CodeKey{Version: "concurrent-old", Secret: bytes.Repeat([]byte{0x35}, 32)}
	newKey := rewards.CodeKey{Version: "concurrent-new", Secret: bytes.Repeat([]byte{0x36}, 32)}
	oldKeyring, err := rewards.NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := rewards.NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	services := []*rewards.Service{
		rewards.NewService(pool, rewardscredits.New(creditService), oldKeyring),
		rewards.NewService(pool, rewardscredits.New(creditService), rotatedKeyring),
	}
	command := ports.CreateRewardCodeCommand{
		DisplayName: "并发轮换奖励", Code: "CONCURRENT-ROTATE-2026", CreditAmount: 16,
		IdempotencyKey: "reward-code-concurrent-rotation", ActorUserID: 1,
	}
	results := make([]ports.CreatedRewardCode, len(services))
	errorsByService := make([]error, len(services))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *rewards.Service) {
			defer wait.Done()
			<-start
			results[index], errorsByService[index] = service.CreateRewardCode(t.Context(), command)
		}(index, service)
	}
	close(start)
	wait.Wait()

	replayed, plaintextResponses := 0, 0
	for index, createError := range errorsByService {
		if createError != nil {
			t.Fatalf("concurrent rotation create %d error = %v", index, createError)
		}
		if results[index].Replayed {
			replayed++
		}
		if results[index].Plaintext != "" {
			plaintextResponses++
		}
	}
	if results[0].RewardCode.ID != results[1].RewardCode.ID || replayed != 1 || plaintextResponses != 1 {
		t.Fatalf("concurrent rotation results = %+v, want one create and one replay", results)
	}
}

func TestRewardCodeCreateSerializesSamePlaintextAcrossRollingKeyrings(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	oldKey := rewards.CodeKey{Version: "rolling-old", Secret: bytes.Repeat([]byte{0x41}, 32)}
	newKey := rewards.CodeKey{Version: "rolling-new", Secret: bytes.Repeat([]byte{0x42}, 32)}
	oldKeyring, err := rewards.NewCodeKeyring(oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	newKeyring, err := rewards.NewCodeKeyring(newKey, &oldKey)
	if err != nil {
		t.Fatal(err)
	}
	services := []*rewards.Service{
		rewards.NewService(pool, rewardscredits.New(creditService), oldKeyring),
		rewards.NewService(pool, rewardscredits.New(creditService), newKeyring),
	}
	errorsByService := make([]error, len(services))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *rewards.Service) {
			defer wait.Done()
			<-start
			_, errorsByService[index] = service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
				DisplayName: "滚动实例奖励", Code: "ROLLING-CODE-2026", CreditAmount: 10,
				IdempotencyKey: fmt.Sprintf("rolling-instance-%d", index), ActorUserID: 1,
			})
		}(index, service)
	}
	close(start)
	wait.Wait()
	successes, collisions := 0, 0
	for _, createError := range errorsByService {
		switch {
		case createError == nil:
			successes++
		case errors.Is(createError, rewardsdomain.ErrCodeCollision):
			collisions++
		default:
			t.Fatalf("unexpected rolling create error = %v", createError)
		}
	}
	if successes != 1 || collisions != 1 {
		t.Fatalf("rolling creates successes=%d collisions=%d errors=%v", successes, collisions, errorsByService)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM reward_codes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("RewardCode count=%d, want 1", count)
	}
}

func TestRewardCodeRotationIsAtomicReplaySafeAndRevokesOldCode(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "轮换奖励", Code: "OLD-ROTATE-2030", CreditAmount: 12,
		IdempotencyKey: "rotate-management", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "其他奖励", Code: "OTHER-ROTATE-2030", CreditAmount: 1,
		IdempotencyKey: "rotate-other", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	rotations := make([]ports.UpdatedRewardCode, 2)
	rotationErrors := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range rotations {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			rotations[index], rotationErrors[index] = service.UpdateRewardCode(t.Context(), ports.UpdateRewardCodeCommand{
				RewardCodeID: created.RewardCode.ID, DisplayName: "轮换奖励", CreditAmount: 12,
				MaxRedemptions: 0, Code: rewardCodeString("NEW-ROTATE-2030"), ActorUserID: 1,
			})
		}(index)
	}
	close(start)
	wait.Wait()
	replayed, plaintextResponses := 0, 0
	for index, rotationError := range rotationErrors {
		if rotationError != nil {
			t.Fatalf("concurrent UpdateRewardCode() %d error = %v", index, rotationError)
		}
		if rotations[index].Replayed != nil && *rotations[index].Replayed {
			replayed++
		}
		if rotations[index].Plaintext == "NEWROTATE2030" {
			plaintextResponses++
		}
		if rotations[index].RewardCode.MaskedCodeHint != "•••• 2030" {
			t.Fatalf("concurrent rotation %d = %+v", index, rotations[index])
		}
	}
	if replayed != 1 || plaintextResponses != 1 {
		t.Fatalf("concurrent rotations = %+v, want one rotation and one replay", rotations)
	}
	replay, err := service.UpdateRewardCode(t.Context(), ports.UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "轮换奖励", CreditAmount: 12,
		MaxRedemptions: 0, Code: rewardCodeString("NEW-ROTATE-2030"), ActorUserID: 1,
	})
	if err != nil || replay.Replayed == nil || !*replay.Replayed || replay.Plaintext != "" {
		t.Fatalf("rotation replay = %+v error=%v", replay, err)
	}
	if _, err := service.UpdateRewardCode(t.Context(), ports.UpdateRewardCodeCommand{
		RewardCodeID: created.RewardCode.ID, DisplayName: "不应提交", CreditAmount: 99,
		MaxRedemptions: 0, Code: rewardCodeString("OTHER-ROTATE-2030"), ActorUserID: 1,
	}); !errors.Is(err, rewardsdomain.ErrCodeCollision) {
		t.Fatalf("rotation collision error = %v", err)
	}
	var displayName string
	var creditAmount int64
	if err := pool.QueryRow(t.Context(), `SELECT display_name, credit_amount FROM reward_codes WHERE id = $1`,
		created.RewardCode.ID).Scan(&displayName, &creditAmount); err != nil {
		t.Fatal(err)
	}
	if displayName != "轮换奖励" || creditAmount != 12 {
		t.Fatalf("collision committed rule update display_name=%q credit_amount=%d", displayName, creditAmount)
	}
	if other.RewardCode.ID == created.RewardCode.ID {
		t.Fatal("rotation collision fixture reused RewardCode")
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "OLD-ROTATE-2030"}); !errors.Is(err, rewardsdomain.ErrCodeUnavailable) {
		t.Fatalf("old Code Redeem() error = %v", err)
	}
	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "NEW-ROTATE-2030"}); err != nil {
		t.Fatalf("new Code Redeem() error = %v", err)
	}
	var totalActions, rotationActions int
	var sensitiveJournal bool
	if err := pool.QueryRow(t.Context(), `SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE action_type = 'rotate_reward_code'),
  bool_or((metadata || target_snapshot)::TEXT LIKE '%NEWROTATE2030%'
    OR (metadata || target_snapshot)::TEXT LIKE '%OLDROTATE2030%')
FROM reward_admin_actions WHERE reward_code_id_snapshot = $1`, created.RewardCode.ID).Scan(
		&totalActions, &rotationActions, &sensitiveJournal,
	); err != nil {
		t.Fatal(err)
	}
	if totalActions != 2 || rotationActions != 1 || sensitiveJournal {
		t.Fatalf("rotation Journal total=%d rotations=%d sensitive=%t", totalActions, rotationActions, sensitiveJournal)
	}
	var fullDigest, journalText string
	if err := pool.QueryRow(t.Context(), `SELECT encode(code_digest, 'hex') FROM reward_codes WHERE id = $1`,
		created.RewardCode.ID).Scan(&fullDigest); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT string_agg((metadata || target_snapshot)::TEXT, '')
FROM reward_admin_actions WHERE reward_code_id_snapshot = $1`, created.RewardCode.ID).Scan(&journalText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(journalText, fullDigest) {
		t.Fatal("rotation Journal exposed a full Code digest")
	}
}

func TestRewardCodeDeleteRequiresZeroRedemptionsAndPreservesJournalSnapshot(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	deletable, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "误建奖励", Code: "DELETE-CODE-2030", CreditAmount: 3,
		IdempotencyKey: "delete-code", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := service.DeleteRewardCode(t.Context(), ports.DeleteRewardCodeCommand{
		RewardCodeID: deletable.RewardCode.ID, ActorUserID: 1,
	})
	if err != nil || deleted.RewardCodeID != deletable.RewardCode.ID {
		t.Fatalf("DeleteRewardCode() = %+v error=%v", deleted, err)
	}
	var definitions, aliases int
	var journalCodeID *int64
	var journalSnapshotID int64
	var journalSnapshotName string
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT COUNT(*) FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM reward_code_digest_aliases WHERE reward_code_id = $1),
  reward_code_id,
  reward_code_id_snapshot,
  target_snapshot->>'display_name'
FROM reward_admin_actions
WHERE reward_code_id_snapshot = $1 AND action_type = 'delete_reward_code'`, deletable.RewardCode.ID).Scan(
		&definitions, &aliases, &journalCodeID, &journalSnapshotID, &journalSnapshotName,
	); err != nil {
		t.Fatal(err)
	}
	if definitions != 0 || aliases != 0 || journalCodeID != nil ||
		journalSnapshotID != deletable.RewardCode.ID || journalSnapshotName != "误建奖励" {
		t.Fatalf("delete facts definitions=%d aliases=%d code_id=%v snapshot_id=%d name=%q",
			definitions, aliases, journalCodeID, journalSnapshotID, journalSnapshotName)
	}

	redeemed, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "已兑奖励", Code: "REDEEMED-DELETE-2030", CreditAmount: 4,
		IdempotencyKey: "redeemed-delete", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)
	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "REDEEMED-DELETE-2030"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteRewardCode(t.Context(), ports.DeleteRewardCodeCommand{
		RewardCodeID: redeemed.RewardCode.ID, ActorUserID: 1,
	}); !errors.Is(err, rewardsdomain.ErrRewardCodeRedeemed) {
		t.Fatalf("redeemed DeleteRewardCode() error = %v", err)
	}
}

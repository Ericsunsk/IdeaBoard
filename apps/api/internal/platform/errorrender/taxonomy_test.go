package errorrender

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestConstructorsCarryTaxonomy(t *testing.T) {
	err := BadRequest("bad_input", "输入无效")
	if err.Code() != "bad_input" || err.HTTPStatus() != http.StatusBadRequest {
		t.Fatalf("taxonomy = %+v", err)
	}
	if err.Blame() != BlameCaller || err.Retryable() {
		t.Fatalf("blame/retryable = %q/%v", err.Blame(), err.Retryable())
	}
	if err.Error() != "输入无效" {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestWrapPreservesIsChain(t *testing.T) {
	sentinel := errors.New("boom")
	wrapped := Wrap("wrapped_boom", http.StatusBadGateway, BlameProvider, sentinel, "上游失败")
	if !errors.Is(wrapped, sentinel) {
		t.Fatal("wrapped error should match errors.Is against the cause")
	}
	if CodeOf(wrapped) != "wrapped_boom" || HTTPStatusOf(wrapped) != http.StatusBadGateway {
		t.Fatalf("taxonomy lost through wrap: %+v", wrapped)
	}
}

func TestAsTraversesJoin(t *testing.T) {
	classified := Conflict("state_conflict", "状态冲突")
	joined := errors.Join(classified, errors.New("raw provider body sk_test_secret"))
	got, ok := As(joined)
	if !ok || got.Code() != "state_conflict" {
		t.Fatalf("As(join) = %+v, %v", got, ok)
	}
	if CodeOf(joined) != "state_conflict" || HTTPStatusOf(joined) != http.StatusConflict {
		t.Fatal("helpers should see through errors.Join")
	}
}

func TestUnclassifiedDefaults(t *testing.T) {
	plain := errors.New("plain")
	if _, ok := As(plain); ok {
		t.Fatal("plain error must not classify")
	}
	if CodeOf(plain) != "" || HTTPStatusOf(plain) != 0 {
		t.Fatal("unclassified helpers must return zero values")
	}
	if !RetryableOf(plain) {
		t.Fatal("unclassified errors default to retryable (fail closed toward retry)")
	}
	if RetryableOf(fmt.Errorf("wrap: %w", plain)) {
		// %w chain without taxonomy stays unclassified: still retryable.
	} else {
		t.Fatal("wrapped plain error should stay retryable")
	}
	transient := ServiceUnavailable("downstream_down", BlameProvider, "下游暂不可用").WithRetryable(true)
	if !RetryableOf(transient) {
		t.Fatal("WithRetryable(true) should stick")
	}
	terminal := Conflict("already_done", "已完成")
	if RetryableOf(terminal) {
		t.Fatal("conflicts default to non-retryable")
	}
}

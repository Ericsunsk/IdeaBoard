package hints_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ideaboard/api/internal/payments/application/hints"
	"ideaboard/api/internal/payments/ports"
)

type delivery struct {
	userID int64
	event  ports.PaymentEvent
}

type recordingPublisher struct{ deliveries []delivery }

func (p *recordingPublisher) Publish(userID int64, event ports.PaymentEvent) {
	p.deliveries = append(p.deliveries, delivery{userID, event})
}

type audience struct {
	managers []int64
	err      error
	accounts []int64
	ctx      context.Context
}

func (a *audience) ListBillingAccountManagerIDs(ctx context.Context, accountID int64) ([]int64, error) {
	a.ctx = ctx
	a.accounts = append(a.accounts, accountID)
	return a.managers, a.err
}

func TestPublishUsesCurrentManagersOrOnlyOperator(t *testing.T) {
	for _, tt := range []struct {
		name     string
		managers []int64
		err      error
		account  int64
		operator int64
		want     []int64
	}{
		{name: "managers exclude departed operator", managers: []int64{8, 9}, account: 101, operator: 7, want: []int64{8, 9}},
		{name: "lookup failure", managers: []int64{8}, err: errors.New("unavailable"), account: 101, operator: 7, want: []int64{7}},
		{name: "empty audience", account: 101, operator: 7, want: []int64{7}},
		{name: "invalid audience", managers: []int64{0, -8}, account: 101, operator: 7, want: []int64{7}},
		{name: "valid unique managers", managers: []int64{0, 8, -1, 8, 9, 9}, account: 101, operator: 7, want: []int64{8, 9}},
		{name: "no persisted payer", managers: []int64{8}, operator: 7, want: []int64{7}},
		{name: "no valid fallback", account: 101, operator: -7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := &recordingPublisher{}
			current := &audience{managers: tt.managers, err: tt.err}
			publisher := hints.NewPublisher(events, current)
			event := ports.PaymentEvent{BillingAccountID: tt.account, PublicToken: "token", Status: ports.PaymentEventCredited}
			publisher.Publish(t.Context(), tt.operator, event)
			var got []int64
			for _, d := range events.deliveries {
				got = append(got, d.userID)
				if d.event != event {
					t.Fatalf("hint changed: %+v, want %+v", d.event, event)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("recipients = %v, want %v", got, tt.want)
			}
			if tt.account > 0 {
				if !reflect.DeepEqual(current.accounts, []int64{tt.account}) || current.ctx != t.Context() {
					t.Fatalf("audience lookup must preserve payer and caller context: %+v", current)
				}
			} else if len(current.accounts) != 0 {
				t.Fatalf("missing payer triggered lookup: %v", current.accounts)
			}
		})
	}
}

func TestPublishReadsAudienceForEachDelivery(t *testing.T) {
	events := &recordingPublisher{}
	current := &audience{managers: []int64{8}}
	publisher := hints.NewPublisher(events, current)
	event := ports.PaymentEvent{BillingAccountID: 101, Status: ports.PaymentEventRefundUpdated}
	publisher.Publish(t.Context(), 7, event)
	current.managers = []int64{9}
	publisher.Publish(t.Context(), 7, event)
	if !reflect.DeepEqual(events.deliveries, []delivery{{8, event}, {9, event}}) {
		t.Fatalf("hints used stale audience: %+v", events.deliveries)
	}
}

func TestPublishWithoutAudienceOrTransport(t *testing.T) {
	events := &recordingPublisher{}
	event := ports.PaymentEvent{BillingAccountID: 101, Status: ports.PaymentEventDisputeUpdated}
	hints.NewPublisher(events, nil).Publish(t.Context(), 7, event)
	if !reflect.DeepEqual(events.deliveries, []delivery{{7, event}}) {
		t.Fatalf("missing audience recipients: %+v", events.deliveries)
	}
	current := &audience{managers: []int64{8}}
	hints.NewPublisher(nil, current).Publish(t.Context(), 7, event)
	var absent *hints.Publisher
	absent.Publish(t.Context(), 7, event)
	if len(current.accounts) != 0 {
		t.Fatal("disabled transport queried audience")
	}
}

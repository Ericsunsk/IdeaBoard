package events_test

import (
	"context"
	"testing"

	"ideaboard/api/internal/events"
	eventinterfaces "ideaboard/api/internal/runtime/interfaces/events"
)

func TestConfigureUserEventFanoutRequiresDependencies(t *testing.T) {
	err := eventinterfaces.ConfigureUserEventFanout(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when user hub or payment service is nil")
	}

	userHub := events.NewUserEventHub()
	err = eventinterfaces.ConfigureUserEventFanout(context.Background(), nil, userHub, nil)
	if err == nil {
		t.Fatal("expected error when payment service is nil")
	}
}

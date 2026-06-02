package events

import "testing"

func TestNormalizeNATSJetStreamConfig(t *testing.T) {
	cfg := normalizeNATSJetStreamConfig(NATSJetStreamConfig{
		URL:           " nats://127.0.0.1:4222 ",
		StreamName:    "IdeaBoard.Events",
		SubjectPrefix: " idea.board.* ",
		ConsumerName:  "api.instance/1",
	})

	if cfg.URL != "nats://127.0.0.1:4222" {
		t.Fatalf("URL=%q, want trimmed URL", cfg.URL)
	}
	if cfg.StreamName != "IdeaBoard_Events" {
		t.Fatalf("StreamName=%q, want IdeaBoard_Events", cfg.StreamName)
	}
	if cfg.SubjectPrefix != "idea.board" {
		t.Fatalf("SubjectPrefix=%q, want idea.board", cfg.SubjectPrefix)
	}
	if cfg.ConsumerName != "api_instance_1" {
		t.Fatalf("ConsumerName=%q, want api_instance_1", cfg.ConsumerName)
	}
	if cfg.Retention != defaultNATSRetention {
		t.Fatalf("Retention=%s, want %s", cfg.Retention, defaultNATSRetention)
	}
}

func TestTaskStatusSubject(t *testing.T) {
	if got := taskStatusSubject(" idea.board "); got != "idea.board.task.status" {
		t.Fatalf("taskStatusSubject=%q, want idea.board.task.status", got)
	}
}

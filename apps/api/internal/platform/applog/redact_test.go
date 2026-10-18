package applog

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func logThroughRedactor(t *testing.T, appEnv string, log func(*slog.Logger)) string {
	t.Helper()
	var buf bytes.Buffer
	handler := newHandler(&buf, appEnv, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	log(logger)
	return buf.String()
}

func TestRedactingHandlerMasksSensitiveKeys(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		output := logThroughRedactor(t, appEnv, func(logger *slog.Logger) {
			logger.Info("connecting",
				"host", "127.0.0.1",
				"DBPassword", "super-secret-value",
				"auth_internal_key", "shared-key",
				"accessToken", "bearer-abc",
				"user_id", 42,
			)
		})
		for _, leaked := range []string{"super-secret-value", "shared-key", "bearer-abc"} {
			if strings.Contains(output, leaked) {
				t.Fatalf("env %q log leaks sensitive value %q: %s", appEnv, leaked, output)
			}
		}
		for _, want := range []string{"127.0.0.1", "42", "[REDACTED]"} {
			if !strings.Contains(output, want) {
				t.Fatalf("env %q log dropped non-sensitive %q: %s", appEnv, want, output)
			}
		}
	}
}

func TestRedactingHandlerMasksDashVariantKeys(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		output := logThroughRedactor(t, appEnv, func(logger *slog.Logger) {
			logger.Info("connecting",
				"host", "127.0.0.1",
				"api-key", "sk-live-123",
				"private-key", "pem-body",
				"pwd", "hunter2",
				"user_id", 42,
			)
		})
		for _, leaked := range []string{"sk-live-123", "pem-body", "hunter2"} {
			if strings.Contains(output, leaked) {
				t.Fatalf("env %q log leaks sensitive value %q: %s", appEnv, leaked, output)
			}
		}
		for _, want := range []string{"127.0.0.1", "42", "[REDACTED]"} {
			if !strings.Contains(output, want) {
				t.Fatalf("env %q log dropped non-sensitive %q: %s", appEnv, want, output)
			}
		}
	}
}

func TestRedactingHandlerScrubsSecretAndAPIKeyPairs(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			output := logThroughRedactor(t, appEnv, func(logger *slog.Logger) {
				logger.Error("call failed", "detail", "login secret=hunter2 api_key=sk-123 host=db")
			})
			for _, secret := range []string{"hunter2", "sk-123"} {
				if strings.Contains(output, secret) {
					t.Fatalf("log leaked credential %q: %s", secret, output)
				}
			}
			if strings.Count(output, redactedValue) != 2 || !strings.Contains(output, "host=db") {
				t.Fatalf("expected two redactions and diagnostic context: %s", output)
			}
		})
	}
}

func TestRedactingHandlerScrubsEmbeddedCredentialPairs(t *testing.T) {
	output := logThroughRedactor(t, "production", func(logger *slog.Logger) {
		logger.Error("dial failed", "err", "connect postgres://ideaboard:hunter2@db:5432/app?sslmode=require failed")
	})
	if strings.Contains(output, "hunter2") {
		t.Fatalf("log leaks embedded password: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("log did not redact embedded password: %s", output)
	}
	if !strings.Contains(output, "ideaboard") {
		t.Fatalf("log dropped the username while redacting: %s", output)
	}
}

func TestRedactingHandlerKeepsPlainURLsIntact(t *testing.T) {
	output := logThroughRedactor(t, "production", func(logger *slog.Logger) {
		logger.Info("fetch", "url", "see https://example.com/a?x=1 and postgres://u:p@h/db")
	})
	if !strings.Contains(output, "https://example.com/a?x=1") {
		t.Fatalf("log mangled a plain URL: %s", output)
	}
	if strings.Contains(output, ":p@h") {
		t.Fatalf("log leaks second URL password: %s", output)
	}
}

func TestRedactingHandlerRedactsGroupsAndWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := newHandler(&buf, "development", &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler).With("api_key", "key-123")
	logger.Info("grouped", slog.Group("auth", "token", "tok-456", "user", "u1"))
	output := buf.String()
	for _, leaked := range []string{"key-123", "tok-456"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("log leaks grouped sensitive value %q: %s", leaked, output)
		}
	}
	if !strings.Contains(output, "u1") {
		t.Fatalf("log dropped non-sensitive grouped value: %s", output)
	}
}

func TestRedactingHandlerPassesThroughOrdinaryAttrs(t *testing.T) {
	output := logThroughRedactor(t, "production", func(logger *slog.Logger) {
		logger.Info("request", "request_id", "req-1", "route", "/api/health", "status", 200)
	})
	for _, want := range []string{"req-1", "/api/health", "200"} {
		if !strings.Contains(output, want) {
			t.Fatalf("log dropped ordinary attr %q: %s", want, output)
		}
	}
	if strings.Contains(output, "[REDACTED]") {
		t.Fatalf("log redacted ordinary attrs: %s", output)
	}
}

func TestRedactingHandlerScrubsCredentialPairs(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			output := logThroughRedactor(t, appEnv, func(logger *slog.Logger) {
				logger.Error("dial failed", "detail", "password=hunter2 token=abc123 password=other-secret host=db")
			})
			for _, secret := range []string{"hunter2", "abc123", "other-secret"} {
				if strings.Contains(output, secret) {
					t.Fatalf("log leaked credential %q: %s", secret, output)
				}
			}
			if strings.Count(output, redactedValue) != 3 || !strings.Contains(output, "host=db") {
				t.Fatalf("expected three redactions and diagnostic context: %s", output)
			}
		})
	}
}

func TestRedactingHandlerScrubsErrorValues(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			output := logThroughRedactor(t, appEnv, func(logger *slog.Logger) {
				logger.Error("dial failed", "err", errors.New("connect postgres://app:hunter2@db:5432/app failed"))
			})
			if strings.Contains(output, "hunter2") || !strings.Contains(output, "db:5432/app failed") {
				t.Fatalf("expected redacted error with diagnostic context: %s", output)
			}
		})
	}
}

func TestScrubSensitivePairsBoundaries(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"password= token=abc", "password= token=[REDACTED]"},
		{"password= password=abc", "password= password=[REDACTED]"},
		{"password=[REDACTED]", "password=[REDACTED]"},
		{"PASSWORD='a b' host=db", "PASSWORD=[REDACTED] host=db"},
		{`password="a\" b" host=db`, "password=[REDACTED] host=db"},
		{"password='unterminated value", "password=[REDACTED]"},
		{"token=abc&password=xyz;pwd=def\rhost=db", "token=[REDACTED]&password=[REDACTED];pwd=[REDACTED]\rhost=db"},
		{"secret=hunter2 host=db", "secret=[REDACTED] host=db"},
		{"SECRET='a b' host=db", "SECRET=[REDACTED] host=db"},
		{"api_key=sk-123&x=1", "api_key=[REDACTED]&x=1"},
		{"api-key=sk-123 host=db", "api-key=[REDACTED] host=db"},
		{"apikey=sk-123;host=db", "apikey=[REDACTED];host=db"},
		{"İ 错误 PASSWORD=abc host=db", "İ 错误 PASSWORD=[REDACTED] host=db"},
		{"\xff token=abc", "\xff token=[REDACTED]"},
		{"postgres://u:@db postgres://u:secret@db", "postgres://u:@db postgres://u:[REDACTED]@db"},
		{"postgres://u:p@ssword@db/app", "postgres://u:[REDACTED]@db/app"},
		{"postgres://u:p@ssword@db/app https://x:other@host?q=u@example.com", "postgres://u:[REDACTED]@db/app https://x:[REDACTED]@host?q=u@example.com"},
		{"https://example.com/path/a:b@c", "https://example.com/path/a:b@c"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, _ := scrubSensitivePairs(tt.input)
			if got != tt.want {
				t.Fatalf("scrubSensitivePairs = %q, want %q", got, tt.want)
			}
			if again, _ := scrubSensitivePairs(got); again != got {
				t.Fatalf("scrubbing twice changed the result: %q", again)
			}
		})
	}
}

func TestRedactingHandlerSensitiveGroups(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			output := logThroughRedactor(t, appEnv, func(logger *slog.Logger) {
				logger.Info("group", slog.Group("authorization", "value", "Bearer direct-secret"))
				logger.With("request_id", "req-1").WithGroup("authorization").
					With("value", "Bearer bound-secret").WithGroup("nested").
					Info("group", "value", "Bearer nested-secret")
				logger.Info("ordinary", "value", "visible")
			})
			for _, secret := range []string{"direct-secret", "bound-secret", "nested-secret"} {
				if strings.Contains(output, secret) {
					t.Fatalf("group leaks %q: %s", secret, output)
				}
			}
			if !strings.Contains(output, "req-1") || !strings.Contains(output, "visible") {
				t.Fatalf("group state leaked into parent logger: %s", output)
			}
		})
	}
}

type countingLogValue struct{ calls int }

func (v *countingLogValue) LogValue() slog.Value {
	v.calls++
	return slog.StringValue("postgres://u:hunter2@db/app")
}

func TestRedactingHandlerResolvesLogValuerOnce(t *testing.T) {
	value := &countingLogValue{}
	output := logThroughRedactor(t, "production", func(logger *slog.Logger) {
		logger.Info("value", "detail", value)
	})
	if value.calls != 1 || strings.Contains(output, "hunter2") {
		t.Fatalf("LogValuer calls = %d; output: %s", value.calls, output)
	}
}

func TestRedactingHandlerScrubsMessage(t *testing.T) {
	output := logThroughRedactor(t, "production", func(logger *slog.Logger) {
		logger.Error("connect postgres://app:hunter2@db failed password=abc123")
	})
	if strings.Contains(output, "hunter2") || strings.Contains(output, "abc123") {
		t.Fatalf("log message leaks credentials: %s", output)
	}
}

// Package testenv converges per-test environment-variable boilerplate into
// one seam: trimmed reads with explicit skip-vs-fatal semantics. Callers
// must preserve the original behavior of their call site: tests that skip
// without an external dependency use GetOrSkip, tests that cannot proceed
// without the value use Require.
package testenv

import (
	"os"
	"strings"
	"testing"
)

// Get returns the trimmed value of key, or "" when unset or blank.
func Get(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// Require returns the trimmed value of key, failing the test when it is
// unset or blank. An optional message overrides the default fatal message.
func Require(t testing.TB, key string, message ...string) string {
	t.Helper()
	if value := Get(key); value != "" {
		return value
	}
	msg := "environment variable " + key + " is required"
	if len(message) > 0 && strings.TrimSpace(message[0]) != "" {
		msg = message[0]
	}
	t.Fatal(msg)
	return ""
}

// GetOrSkip returns the trimmed value of key, skipping the test when it is
// unset or blank. An optional message overrides the default skip message.
func GetOrSkip(t testing.TB, key string, message ...string) string {
	t.Helper()
	if value := Get(key); value != "" {
		return value
	}
	msg := "set " + key + " to run this test"
	if len(message) > 0 && strings.TrimSpace(message[0]) != "" {
		msg = message[0]
	}
	t.Skip(msg)
	return ""
}

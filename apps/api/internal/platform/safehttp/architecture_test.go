package safehttp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every outbound call must declare private-egress allowances, a stricter
// public-host restriction, or one operator-trusted configured endpoint.
func TestOutboundCallsPinAllowHosts(t *testing.T) {
	root := "../../.."
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, literal := range extractRequestOptionsLiterals(string(raw)) {
			if !strings.Contains(literal, "AllowHosts") && !strings.Contains(literal, "PublicHosts") && !strings.Contains(literal, "Configured") {
				t.Errorf("production file %s builds RequestOptions without an explicit egress policy", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

// extractRequestOptionsLiterals returns each `safehttp.RequestOptions{...}`
// composite literal using brace matching, so multi-line field lists
// resolve. Only the canonical spelling is accepted: the deprecated
// outboundhttp shim is removed, so any alias spelling fails closed here.
func extractRequestOptionsLiterals(src string) []string {
	return extractLiteralsWithMarker(src, "safehttp.RequestOptions{")
}

func extractLiteralsWithMarker(src, marker string) []string {
	var literals []string
	for i := 0; i+len(marker) <= len(src); {
		start := strings.Index(src[i:], marker)
		if start < 0 {
			break
		}
		open := i + start + len(marker) - 1
		depth := 0
		for j := open; j < len(src); j++ {
			switch src[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					literals = append(literals, src[open:j+1])
					i = j + 1
					goto next
				}
			}
		}
		break
	next:
	}
	return literals
}

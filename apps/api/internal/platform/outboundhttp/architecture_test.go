package outboundhttp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every outbound call must pin egress through the safehttp SSRF allowlist.
// A RequestOptions literal without AllowHosts silently widens egress.
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
			if !strings.Contains(literal, "AllowHosts") {
				t.Errorf("production file %s builds outboundhttp.RequestOptions without AllowHosts", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

// extractRequestOptionsLiterals returns each `outboundhttp.RequestOptions{...}`
// composite literal using brace matching, so multi-line field lists resolve.
func extractRequestOptionsLiterals(src string) []string {
	const marker = "outboundhttp.RequestOptions{"
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

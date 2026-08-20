package schemaassembly

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Manifest struct {
	Version   int        `json:"version"`
	Output    string     `json:"output"`
	Fragments []Fragment `json:"fragments"`
}

type Fragment struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	StartLine int      `json:"start_line,omitempty"`
	EndLine   int      `json:"end_line,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

var requiredFragmentNames = []string{
	"identity", "credits", "account_pool", "platform", "tasks", "media", "library", "usage",
	"workflow", "boards", "rewards", "billing", "payments", "integrity", "bootstrap",
}

func LoadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read schema manifest %q: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse schema manifest %q: %w", path, err)
	}
	return manifest, nil
}

func Assemble(manifest Manifest, root string) ([]byte, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	if isCanonicalManifest(manifest) {
		if err := validateRegisteredFragments(manifest, root); err != nil {
			return nil, err
		}
	}
	seenObjects := make(map[string]string)
	var output strings.Builder
	for _, fragment := range manifest.Fragments {
		raw, err := readFragment(root, fragment)
		if err != nil {
			return nil, fmt.Errorf("fragment %q: %w", fragment.Name, err)
		}
		for _, object := range declaredObjects(string(raw)) {
			if previous, exists := seenObjects[object]; exists {
				return nil, fmt.Errorf("duplicate schema object %q in fragments %q and %q", object, previous, fragment.Name)
			}
			seenObjects[object] = fragment.Name
		}
		output.Write(raw)
	}
	return []byte(strings.TrimRight(output.String(), "\n") + "\n"), nil
}

func validateRegisteredFragments(manifest Manifest, root string) error {
	registered := make(map[string]struct{}, len(manifest.Fragments))
	for _, fragment := range manifest.Fragments {
		registered[filepath.Clean(fragment.Path)] = struct{}{}
	}
	paths, err := filepath.Glob(filepath.Join(root, "fragments", "*.sql"))
	if err != nil {
		return fmt.Errorf("scan schema fragments: %w", err)
	}
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve schema fragment %q: %w", path, err)
		}
		if _, exists := registered[relative]; !exists {
			return fmt.Errorf("schema fragment %q is not registered in manifest", relative)
		}
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported schema manifest version %d", manifest.Version)
	}
	if strings.TrimSpace(manifest.Output) == "" {
		return errors.New("schema manifest output is required")
	}
	if len(manifest.Fragments) == 0 {
		return errors.New("schema manifest must declare at least one fragment")
	}
	positions := make(map[string]int, len(manifest.Fragments))
	for index, fragment := range manifest.Fragments {
		if strings.TrimSpace(fragment.Name) == "" {
			return fmt.Errorf("fragment %d has empty name", index)
		}
		if _, exists := positions[fragment.Name]; exists {
			return fmt.Errorf("duplicate manifest fragment %q", fragment.Name)
		}
		positions[fragment.Name] = index
		if strings.TrimSpace(fragment.Path) == "" {
			return fmt.Errorf("fragment %q has empty path", fragment.Name)
		}
		if fragment.StartLine < 0 || fragment.EndLine < 0 || (fragment.StartLine == 0) != (fragment.EndLine == 0) || (fragment.StartLine > 0 && fragment.EndLine < fragment.StartLine) {
			return fmt.Errorf("fragment %q has invalid line range", fragment.Name)
		}
	}
	if isCanonicalManifest(manifest) {
		for _, required := range requiredFragmentNames {
			if _, exists := positions[required]; !exists {
				return fmt.Errorf("schema manifest is missing required fragment %q", required)
			}
		}
	}
	for index, fragment := range manifest.Fragments {
		for _, dependency := range fragment.DependsOn {
			dependencyIndex, exists := positions[dependency]
			if !exists {
				return fmt.Errorf("fragment %q depends on unknown fragment %q", fragment.Name, dependency)
			}
			if dependencyIndex >= index {
				return fmt.Errorf("fragment %q depends on %q, which is not earlier in manifest order", fragment.Name, dependency)
			}
		}
	}
	return nil
}

func isCanonicalManifest(manifest Manifest) bool {
	for _, fragment := range manifest.Fragments {
		for _, required := range requiredFragmentNames {
			if fragment.Name == required {
				return true
			}
		}
	}
	return false
}

func readFragment(root string, fragment Fragment) ([]byte, error) {
	cleanPath := filepath.Clean(fragment.Path)
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("path %q escapes manifest root", fragment.Path)
	}
	path := filepath.Join(root, cleanPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", fragment.Path, err)
	}
	if fragment.StartLine == 0 {
		return raw, nil
	}
	lines := strings.SplitAfter(string(raw), "\n")
	if fragment.StartLine > len(lines) || fragment.EndLine > len(lines) {
		return nil, fmt.Errorf("line range %d-%d is outside %q (%d lines)", fragment.StartLine, fragment.EndLine, fragment.Path, len(lines))
	}
	return []byte(strings.Join(lines[fragment.StartLine-1:fragment.EndLine], "")), nil
}

var objectPattern = regexp.MustCompile(`(?im)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?((?:(?:UNIQUE\s+)?INDEX)|TABLE|TRIGGER|FUNCTION)\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z_][a-zA-Z0-9_$]*)`)

func declaredObjects(sql string) []string {
	matches := objectPattern.FindAllStringSubmatch(sql, -1)
	objects := make([]string, 0, len(matches))
	for _, match := range matches {
		objects = append(objects, strings.ToLower(match[1])+":"+strings.ToLower(match[2]))
	}
	return objects
}

// ValidateManifest is exported for CI and focused contract tests.
func ValidateManifest(manifest Manifest) error { return validateManifest(manifest) }

// FragmentNames returns the explicit order, useful for diagnostics and tests.
func FragmentNames(manifest Manifest) []string {
	result := make([]string, len(manifest.Fragments))
	for index, fragment := range manifest.Fragments {
		result[index] = fragment.Name
	}
	return result
}

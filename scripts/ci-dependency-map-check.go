package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

type edge struct {
	Input    string `json:"input"`
	Consumer string `json:"consumer"`
	Evidence string `json:"evidence"`
}

var quoted = regexp.MustCompile("['\"]([^'\"\n]+)['\"]")
var workflowInput = regexp.MustCompile(`(?:^|[\s"'])(?:\./)?(apps/[A-Za-z0-9_./-]+)`)

func main() {
	rootArg := flag.String("root", "../..", "repository root")
	jsonPath := flag.String("json", "", "write discovered dependency map")
	flag.Parse()
	root, err := filepath.Abs(*rootArg)
	must(err)
	inventory, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	must(err)
	files := strings.Split(strings.TrimSuffix(string(inventory), "\x00"), "\x00")
	tracked := map[string]bool{}
	for _, file := range files {
		tracked[file] = true
	}
	edges := map[string]edge{}
	add := func(input, consumer, evidence string) {
		if input == "" || consumer == "" {
			return
		}
		key := input + "\x00" + consumer
		edges[key] = edge{input, consumer, evidence}
	}
	ci := load(filepath.Join(root, ".github/workflows/ci.yml"))
	jobs, _ := ci["jobs"].(map[string]any)
	for key, raw := range jobs {
		job, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		uses, _ := job["uses"].(string)
		if !strings.HasPrefix(uses, "./.github/workflows/") {
			continue
		}
		lane := strings.ReplaceAll(key, "-", "_")
		workflow := strings.TrimPrefix(uses, "./")
		document := load(filepath.Join(root, workflow))
		seenActions := map[string]bool{}
		var inspectAction func(string)
		inspectAction = func(action string) {
			if !strings.HasPrefix(action, "./.github/actions/") || seenActions[action] {
				return
			}
			seenActions[action] = true
			input := strings.TrimPrefix(action, "./") + "/action.yml"
			add(input, lane, workflow)
			definition := load(filepath.Join(root, input))
			walkUses(definition, inspectAction)
			walkToolchainFiles(definition, func(path string) {
				if tracked[path] {
					add(path, lane, input)
				}
			})
		}
		walkUses(document, inspectAction)
		walkToolchainFiles(document, func(path string) {
			if tracked[path] {
				add(path, lane, workflow)
			}
		})
	}
	for _, file := range files {
		owner := app(file)
		if owner == "" || !(strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".ts") || strings.HasSuffix(file, ".tsx")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, file))
		must(err)
		refs := []string{}
		if strings.HasSuffix(file, ".go") {
			parsed, err := parser.ParseFile(token.NewFileSet(), file, data, 0)
			must(err)
			ast.Inspect(parsed, func(node ast.Node) bool {
				if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
					value, err := strconv.Unquote(literal.Value)
					if err == nil {
						refs = append(refs, value)
					}
				}
				// filepath.Join(root,"apps","web",...) and relative fixture paths.
				if call, ok := node.(*ast.CallExpr); ok {
					selector, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || selector.Sel.Name != "Join" {
						return true
					}
					parts := []string{}
					for _, arg := range call.Args {
						literal, ok := arg.(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							parts = nil
							continue
						}
						value, err := strconv.Unquote(literal.Value)
						if err == nil {
							parts = append(parts, value)
						}
					}
					if len(parts) > 0 {
						refs = append(refs, filepath.ToSlash(filepath.Join(parts...)))
					}
				}
				return true
			})
		} else {
			// Conservatively discover literal paths without a TypeScript toolchain.
			for _, match := range quoted.FindAllSubmatch(data, -1) {
				refs = append(refs, string(match[1]))
			}
		}
		for _, ref := range refs {
			if strings.ContainsAny(ref, "\n\r\x00") {
				continue
			}
			candidates := []string{}
			if strings.HasPrefix(ref, "apps/") || strings.HasPrefix(ref, "contracts/") {
				candidates = append(candidates, filepath.ToSlash(filepath.Clean(ref)))
			}
			if strings.HasPrefix(ref, "../") || strings.HasPrefix(ref, "./") {
				candidates = append(candidates, filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(file), ref))))
			}
			for _, candidate := range candidates {
				targets := []string{}
				if tracked[candidate] {
					targets = append(targets, candidate)
				} else if app(candidate) != "" || strings.HasPrefix(candidate, "contracts/") {
					// An explicit cross-context subdirectory can name a fixture tree.
					// Root/context paths also locate processes or temporary workspaces;
					// their literals alone do not prove consumption of all descendants.
					for _, target := range files {
						if strings.HasPrefix(target, candidate+"/") {
							targets = append(targets, target)
						}
					}
				}
				for _, target := range targets {
					targetOwner := app(target)
					if targetOwner != "" && targetOwner != owner || strings.HasPrefix(target, "contracts/") {
						add(target, owner, file)
					}
				}
			}
		}
	}
	result := make([]edge, 0, len(edges))
	for _, item := range edges {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Input == result[j].Input {
			return result[i].Consumer < result[j].Consumer
		}
		return result[i].Input < result[j].Input
	})
	if *jsonPath != "" {
		data, err := json.MarshalIndent(map[string]any{"schema_version": 1, "evidence": "tracked workflow uses and literal cross-context references", "edges": result}, "", "  ")
		must(err)
		must(os.WriteFile(*jsonPath, append(data, '\n'), 0600))
	}
	cache := map[string]map[string]string{}
	failures := []string{}
	for _, item := range result {
		output, ok := cache[item.Input]
		if !ok {
			command := exec.Command("bash", filepath.Join(root, "scripts/ci-change-classifier.sh"))
			command.Stdin = bytes.NewBufferString(item.Input + "\n")
			raw, err := command.Output()
			must(err)
			output = map[string]string{}
			for _, line := range strings.Split(string(raw), "\n") {
				key, value, ok := strings.Cut(line, "=")
				if ok {
					output[key] = value
				}
			}
			cache[item.Input] = output
		}
		if output[item.Consumer] != "true" {
			failures = append(failures, fmt.Sprintf("%s requires %s via %s", item.Input, item.Consumer, item.Evidence))
		}
	}
	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "CI dependency coverage missing:\n"+strings.Join(failures, "\n"))
		os.Exit(1)
	}
	fmt.Printf("CI dependency map verified: %d discovered edges across %d input paths\n", len(result), len(cache))
}

func app(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 2 && parts[0] == "apps" {
		switch parts[1] {
		case "web", "api", "auth", "realtime":
			return parts[1]
		case "ai-proxy":
			return "ai_proxy"
		}
	}
	return ""
}
func load(path string) map[string]any {
	data, err := os.ReadFile(path)
	must(err)
	document := map[string]any{}
	must(yaml.Unmarshal(data, &document))
	return document
}
func walkUses(value any, visit func(string)) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "uses" {
				if text, ok := child.(string); ok {
					visit(text)
				}
			}
			walkUses(child, visit)
		}
	case []any:
		for _, child := range current {
			walkUses(child, visit)
		}
	}
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ci-dependency-map:", err)
		os.Exit(1)
	}
}

// Toolchain pin files affect every actual caller of their setup action.
func walkToolchainFiles(value any, visit func(string)) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "run" {
				if shell, ok := child.(string); ok {
					for _, match := range workflowInput.FindAllStringSubmatch(shell, -1) {
						visit(match[1])
					}
				}
			}
			if key == "go-version-file" || key == "node-version-file" {
				if path, ok := child.(string); ok && !strings.Contains(path, "${{") {
					visit(path)
				}
			}
			walkToolchainFiles(child, visit)
		}
	case []any:
		for _, child := range current {
			walkToolchainFiles(child, visit)
		}
	}
}

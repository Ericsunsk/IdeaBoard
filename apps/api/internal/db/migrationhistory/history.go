// Package migrationhistory owns migration change discovery and the append-only
// policy shared by schema release and Atlas checks.
package migrationhistory

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Options struct {
	RepositoryRoot string
	MigrationsDir  string
	BaseSHA        string
	HeadSHA        string
}

type change struct {
	status string
	path   string
}

// History retains both commit-range and working-tree changes. A later working
// edit must not conceal an earlier mutation of released history.
type History struct {
	ChangedFiles []string
	CleanBreak   bool
	changes      []change
}

// Inspect discovers changes without enforcing immutability, allowing callers to
// lint changed SQL before reporting a history violation.
func Inspect(ctx context.Context, options Options) (History, error) {
	root := options.RepositoryRoot
	if root == "" {
		root = "."
	}
	run := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, "git", args...)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("migration history git %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
		}
		return output, nil
	}
	output, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return History{}, err
	}
	root = strings.TrimSpace(string(output))
	directory, err := filepath.Abs(options.MigrationsDir)
	if err != nil {
		return History{}, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return History{}, err
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return History{}, fmt.Errorf("migration directory must be inside repository: %s", directory)
	}
	pathspec := filepath.ToSlash(relative) + "/*.sql"
	head := strings.TrimSpace(options.HeadSHA)
	if head == "" {
		head = "HEAD"
	}
	collect := func(revisions ...string) ([]change, error) {
		args := append([]string{"diff", "--no-renames", "--name-status", "-z"}, revisions...)
		output, err := run(append(args, "--", pathspec)...)
		if err != nil || len(output) == 0 {
			return nil, err
		}
		fields := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
		if len(fields)%2 != 0 {
			return nil, fmt.Errorf("invalid migration change records")
		}
		var changes []change
		for i := 0; i < len(fields); i += 2 {
			changes = append(changes, change{status: fields[i], path: fields[i+1]})
		}
		return changes, nil
	}
	history := History{}
	if base := strings.TrimSpace(options.BaseSHA); base != "" {
		if _, err := run("rev-parse", "--verify", base+"^{commit}"); err != nil {
			return History{}, err
		}
		history.changes, err = collect(base, head)
		if err != nil {
			return History{}, err
		}
	}
	working, err := collect(head)
	if err != nil {
		return History{}, err
	}
	history.changes = append(history.changes, working...)
	removed := make(map[string]bool)
	for _, item := range working {
		if item.status == "D" {
			removed[item.path] = true
		}
	}
	output, err = run("ls-files", "--others", "--exclude-standard", "-z", "--", pathspec)
	if err != nil {
		return History{}, err
	}
	for _, path := range strings.Split(string(output), "\x00") {
		if path != "" {
			history.changes = append(history.changes, change{status: "A", path: path})
		}
	}
	candidates := make(map[string]bool)
	for _, item := range history.changes {
		if item.status != "D" && !removed[item.path] {
			candidates[item.path] = true
		}
	}
	for path := range candidates {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil || !info.Mode().IsRegular() {
			return History{}, fmt.Errorf("new migration disappeared: %s", path)
		}
		history.ChangedFiles = append(history.ChangedFiles, path)
	}
	sort.Strings(history.ChangedFiles)
	history.CleanBreak = isCleanBreak(history.changes)
	return history, nil
}

func (history History) Validate() error {
	if history.CleanBreak {
		return nil
	}
	var violations []string
	for _, item := range history.changes {
		if item.status != "A" {
			violations = append(violations, item.status+"\t"+item.path)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("migration history is immutable (released migrations are append-only); invalid changes: %s", strings.Join(violations, ", "))
}

func isCleanBreak(changes []change) bool {
	// A Fresh-DB squash replaces one baseline and may delete its old tails.
	// A coalesced CI range may also add tails strictly after the new baseline.
	var added, deleted int64
	var deletedCount int
	var tails []int64
	for _, item := range changes {
		switch item.status {
		case "A":
			if version, ok := baselineVersion(item.path); ok {
				if added != 0 {
					return false
				}
				added = version
			} else {
				version, ok := migrationVersion(item.path)
				if !ok {
					return false
				}
				tails = append(tails, version)
			}
		case "D":
			if version, ok := baselineVersion(item.path); ok {
				deleted = version
				deletedCount++
			}
		default:
			return false
		}
	}
	if added == 0 || deletedCount != 1 || added <= deleted {
		return false
	}
	for _, tail := range tails {
		if tail <= added {
			return false
		}
	}
	return true
}

func baselineVersion(path string) (int64, bool) {
	const suffix = "_baseline.sql"
	name := filepath.Base(path)
	if !strings.HasSuffix(name, suffix) {
		return 0, false
	}
	text := strings.TrimSuffix(name, suffix)
	if len(text) != 14 {
		return 0, false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	version, err := strconv.ParseInt(text, 10, 64)
	return version, err == nil
}

func migrationVersion(path string) (int64, bool) {
	name := filepath.Base(path)
	underscore := strings.Index(name, "_")
	if underscore <= 0 {
		return 0, false
	}
	text := name[:underscore]
	if len(text) != 14 {
		return 0, false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	version, err := strconv.ParseInt(text, 10, 64)
	return version, err == nil
}

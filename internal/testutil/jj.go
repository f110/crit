package testutil

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// JJStack is a temp Jujutsu repo holding base(main) -> first -> second, with an
// empty working-copy commit on top. Positions are addressable as @--- (base),
// @-- (first), @- (second).
type JJStack struct {
	Dir string
}

// InitJJStack creates a JJStack, skipping the test when jj is unavailable.
func InitJJStack(t *testing.T) JJStack {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed")
	}
	dir := t.TempDir()
	s := JJStack{Dir: dir}
	s.Run(t, "git", "init", ".")
	WriteFile(t, filepath.Join(dir, "app.txt"), "base\n")
	s.Run(t, "file", "track", "app.txt")
	s.Run(t, "commit", "-m", "base")
	s.Run(t, "bookmark", "set", "main", "-r", "@-")
	WriteFile(t, filepath.Join(dir, "one.txt"), "one\n")
	s.Run(t, "commit", "-m", "first")
	WriteFile(t, filepath.Join(dir, "two.txt"), "two\n")
	s.Run(t, "commit", "-m", "second")
	return s
}

// Run executes a jj command in the stack, failing the test on error.
func (s JJStack) Run(t *testing.T, args ...string) string {
	t.Helper()
	full := []string{"--no-pager", "--color", "never",
		"--config", "user.name=Test", "--config", "user.email=test@example.com"}
	full = append(full, args...)
	cmd := exec.Command("jj", full...)
	cmd.Dir = s.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// ChangeIDAt returns the full change id of a revision.
func (s JJStack) ChangeIDAt(t *testing.T, rev string) string {
	t.Helper()
	return s.Run(t, "log", "-r", rev, "--no-graph", "-T", "change_id")
}

// CommitIDAt returns the commit id of a revision.
func (s JJStack) CommitIDAt(t *testing.T, rev string) string {
	t.Helper()
	return s.Run(t, "log", "-r", rev, "--no-graph", "-T", "commit_id")
}

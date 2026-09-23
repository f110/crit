package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestJJStack builds base(main) -> first -> second with @ empty on top.
// Positions are addressable as @--- (base), @-- (first), @- (second).
func initTestJJStack(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed")
	}
	dir := t.TempDir()
	runJJ(t, dir, "git", "init", ".")
	writeStackFile(t, dir, "app.txt", "base\n")
	runJJ(t, dir, "file", "track", "app.txt")
	runJJWithUser(t, dir, "commit", "-m", "base")
	runJJ(t, dir, "bookmark", "set", "main", "-r", "@-")
	writeStackFile(t, dir, "one.txt", "one\n")
	runJJWithUser(t, dir, "commit", "-m", "first")
	writeStackFile(t, dir, "two.txt", "two\n")
	runJJWithUser(t, dir, "commit", "-m", "second")
	return dir
}

func writeStackFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func jjChangeIDAt(t *testing.T, dir, rev string) string {
	t.Helper()
	return runJJ(t, dir, "log", "-r", rev, "--no-graph", "-T", "change_id")
}

func jjCommitIDAt(t *testing.T, dir, rev string) string {
	t.Helper()
	return runJJ(t, dir, "log", "-r", rev, "--no-graph", "-T", "commit_id")
}

func TestResolveJJChangeID(t *testing.T) {
	dir := initTestJJStack(t)
	firstChange := jjChangeIDAt(t, dir, "@--")
	firstCommit := jjCommitIDAt(t, dir, "@--")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"full id", firstChange, firstCommit},
		{"prefix", firstChange[:8], firstCommit},
		{"surrounding whitespace", "  " + firstChange + "\n", firstCommit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveJJChangeID(dir, tt.input)
			if err != nil {
				t.Fatalf("ResolveJJChangeID(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ResolveJJChangeID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveJJChangeID_Errors(t *testing.T) {
	dir := initTestJJStack(t)

	tests := []struct {
		name      string
		input     string
		wantErrIs string
	}{
		{"empty", "", "empty JJ change id"},
		{"unknown", "qqqqqqqqqqqqqqqq", "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveJJChangeID(dir, tt.input)
			if err == nil {
				t.Fatalf("ResolveJJChangeID(%q) = nil error, want error", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErrIs) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErrIs)
			}
		})
	}
}

// TestResolveJJChangeID_SurvivesRewrite is the property the whole change-id
// focus rests on: rewriting a change moves its commit id (and rebases its
// descendants onto new commit ids) while every change id stays put.
func TestResolveJJChangeID_SurvivesRewrite(t *testing.T) {
	dir := initTestJJStack(t)
	firstChange := jjChangeIDAt(t, dir, "@--")
	secondChange := jjChangeIDAt(t, dir, "@-")
	firstCommitBefore := jjCommitIDAt(t, dir, "@--")
	secondCommitBefore := jjCommitIDAt(t, dir, "@-")

	runJJWithUser(t, dir, "describe", "-r", firstChange, "-m", "first rewritten")

	firstAfter, err := ResolveJJChangeID(dir, firstChange)
	if err != nil {
		t.Fatalf("resolving rewritten change: %v", err)
	}
	if firstAfter == firstCommitBefore {
		t.Error("rewritten change kept its commit id; the fixture did not rewrite anything")
	}
	secondAfter, err := ResolveJJChangeID(dir, secondChange)
	if err != nil {
		t.Fatalf("resolving rebased descendant: %v", err)
	}
	if secondAfter == secondCommitBefore {
		t.Error("descendant kept its commit id; the fixture did not rebase it")
	}
}

func TestResolveJJChangeID_RejectsDivergent(t *testing.T) {
	dir := initTestJJStack(t)
	secondChange := jjChangeIDAt(t, dir, "@-")

	// Two rewrites of one change from the same operation diverge it.
	op := runJJ(t, dir, "op", "log", "--no-graph", "-T", "id.short()", "-n", "1")
	runJJWithUser(t, dir, "describe", "-r", secondChange, "-m", "second A")
	runJJWithUser(t, dir, "--at-operation", op, "describe", "-r", secondChange, "-m", "second B")

	_, err := ResolveJJChangeID(dir, secondChange)
	if err == nil {
		t.Fatal("ResolveJJChangeID on a divergent change = nil error, want error")
	}
	if !strings.Contains(err.Error(), "divergent") {
		t.Errorf("error = %q, want it to mention divergence", err)
	}
}

func TestJJChangeIDForCommit(t *testing.T) {
	dir := initTestJJStack(t)
	wantChange := jjChangeIDAt(t, dir, "@--")
	commit := jjCommitIDAt(t, dir, "@--")

	got, err := JJChangeIDForCommit(dir, commit)
	if err != nil {
		t.Fatalf("JJChangeIDForCommit: %v", err)
	}
	if got != wantChange {
		t.Errorf("JJChangeIDForCommit(%s) = %q, want %q", commit, got, wantChange)
	}
}

func TestJJChangeParentCommit(t *testing.T) {
	dir := initTestJJStack(t)
	secondChange := jjChangeIDAt(t, dir, "@-")
	wantParent := jjCommitIDAt(t, dir, "@--")

	got, err := JJChangeParentCommit(dir, secondChange)
	if err != nil {
		t.Fatalf("JJChangeParentCommit: %v", err)
	}
	if got != wantParent {
		t.Errorf("JJChangeParentCommit(%s) = %q, want %q", secondChange, got, wantParent)
	}
}

func TestChangeIDsByCommit(t *testing.T) {
	dir := initTestJJStack(t)
	ids := ChangeIDsByCommit(&JJVCS{}, dir, 20)

	for _, rev := range []string{"@-", "@--"} {
		commit := jjCommitIDAt(t, dir, rev)
		want := jjChangeIDAt(t, dir, rev)
		if got := ids[commit]; got != want {
			t.Errorf("ChangeIDsByCommit[%s] (%s) = %q, want %q", commit, rev, got, want)
		}
	}
}

func TestChangeIDsByCommit_NilForBackendsWithoutChangeIDs(t *testing.T) {
	for _, v := range []VCS{&GitVCS{}, &SaplingVCS{}, nil} {
		if got := ChangeIDsByCommit(v, t.TempDir(), 20); got != nil {
			t.Errorf("ChangeIDsByCommit(%T) = %v, want nil", v, got)
		}
	}
}

func TestJJChangeParentCommit_RejectsMerge(t *testing.T) {
	dir := initTestJJStack(t)
	// A change with two parents: new off both sides of the stack.
	runJJWithUser(t, dir, "new", "@-", "@--", "-m", "merge")
	mergeChange := jjChangeIDAt(t, dir, "@")

	_, err := JJChangeParentCommit(dir, mergeChange)
	if err == nil {
		t.Fatal("JJChangeParentCommit on a merge = nil error, want error")
	}
	if !strings.Contains(err.Error(), "merge commits") {
		t.Errorf("error = %q, want it to mention merge commits", err)
	}
}

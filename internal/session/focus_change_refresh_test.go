package session

import (
	"path/filepath"
	"testing"

	"github.com/tomasz-tomczyk/crit/internal/testutil"
	"github.com/tomasz-tomczyk/crit/internal/vcs"
)

// changeFocusSession opens a session focused on the "first" change of a jj
// stack and returns it alongside the stack and that change's id.
func changeFocusSession(t *testing.T) (*Session, testutil.JJStack, string) {
	t.Helper()
	stack := testutil.InitJJStack(t)
	changeID := stack.ChangeIDAt(t, "@--")
	// OutputDir outside the repo: a review folder inside it would be snapshotted
	// into the change under test by the working-copy switch in rewriteFirstChange.
	s := &Session{
		RepoRoot:  stack.Dir,
		OutputDir: t.TempDir(),
		VCS:       &vcs.JJVCS{},
	}
	f := Focus{
		Kind:        FocusRange,
		VCSChangeID: changeID,
		BaseSHA:     stack.CommitIDAt(t, "@---"),
		HeadSHA:     stack.CommitIDAt(t, "@--"),
		DiffScope:   DiffScopeLayer,
	}
	if err := s.SetFocus(f); err != nil {
		t.Fatalf("SetFocus: %v", err)
	}
	return s, stack, changeID
}

// rewriteFirstChange edits one.txt inside the "first" change, which rewrites
// that commit and rebases "second" on top of the new one.
func rewriteFirstChange(t *testing.T, stack testutil.JJStack, changeID, content string) {
	t.Helper()
	stack.Run(t, "edit", "-r", changeID)
	testutil.WriteFile(t, filepath.Join(stack.Dir, "one.txt"), content)
	stack.Run(t, "status")
}

func TestRefreshChangeIDFocus_FollowsRewrittenChange(t *testing.T) {
	s, stack, changeID := changeFocusSession(t)
	beforeHead := s.Focus.HeadSHA
	beforeKey := focusKeyFor(s.Focus)

	rewriteFirstChange(t, stack, changeID, "one\nrewritten\n")
	s.refreshChangeIDFocus()

	if s.Focus.HeadSHA == beforeHead {
		t.Error("HeadSHA did not follow the rewrite")
	}
	if got := focusKeyFor(s.Focus); got != beforeKey {
		t.Errorf("focus key moved: %q -> %q", beforeKey, got)
	}
	if s.Focus.VCSChangeID != changeID {
		t.Errorf("VCSChangeID = %q, want %q", s.Focus.VCSChangeID, changeID)
	}
	var one *FileEntry
	for _, f := range s.Files {
		if f.Path == "one.txt" {
			one = f
		}
	}
	if one == nil {
		t.Fatalf("one.txt missing from rebuilt file list %+v", s.Files)
	}
	if one.Content != "one\nrewritten\n" {
		t.Errorf("one.txt content = %q, want the rewritten bytes", one.Content)
	}
}

// TestRefreshChangeIDFocus_CarriesRoundState covers the failure this would have
// otherwise: buildFilesForFocus hands back blank entries, so an unguarded swap
// drops the carry-forward inputs and every open comment disappears at the round
// boundary.
func TestRefreshChangeIDFocus_CarriesRoundState(t *testing.T) {
	s, stack, changeID := changeFocusSession(t)

	prev := []Comment{{ID: "c1", StartLine: 1, EndLine: 1, Body: "look here"}}
	s.mu.Lock()
	for _, f := range s.Files {
		if f.Path == "one.txt" {
			f.PreviousContent = f.Content
			f.PreviousComments = prev
		}
	}
	s.mu.Unlock()

	rewriteFirstChange(t, stack, changeID, "inserted\none\n")
	s.refreshChangeIDFocus()

	var one *FileEntry
	for _, f := range s.Files {
		if f.Path == "one.txt" {
			one = f
		}
	}
	if one == nil {
		t.Fatal("one.txt missing from rebuilt file list")
	}
	if one.PreviousContent != "one\n" {
		t.Errorf("PreviousContent = %q, want the pre-rewrite bytes", one.PreviousContent)
	}
	if len(one.PreviousComments) != 1 || one.PreviousComments[0].ID != "c1" {
		t.Errorf("PreviousComments = %+v, want the comment carried over", one.PreviousComments)
	}

	// The carried state is exactly what carry-forward needs to remap the
	// comment past the inserted line.
	s.carryForwardFileComments(one)
	if len(one.Comments) != 1 {
		t.Fatalf("Comments = %+v, want one carried comment", one.Comments)
	}
	if one.Comments[0].StartLine != 2 {
		t.Errorf("StartLine = %d, want 2 after one line was inserted above", one.Comments[0].StartLine)
	}
}

func TestRefreshChangeIDFocus_KeepsPreviousDiffWhenChangeIsGone(t *testing.T) {
	s, stack, changeID := changeFocusSession(t)
	before := s.Focus

	stack.Run(t, "abandon", "-r", changeID)
	s.refreshChangeIDFocus()

	if s.Focus != before {
		t.Errorf("focus changed after the change was abandoned: %+v -> %+v", before, s.Focus)
	}
}

func TestRefreshChangeIDFocus_IgnoresOtherFocusKinds(t *testing.T) {
	stack := testutil.InitJJStack(t)
	tests := []struct {
		name string
		f    Focus
	}{
		{"working tree", Focus{Kind: FocusWorkingTree}},
		{"range without change id", Focus{Kind: FocusRange, BaseSHA: "aaa", HeadSHA: "bbb", DiffScope: DiffScopeLayer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{RepoRoot: stack.Dir, OutputDir: t.TempDir(), VCS: &vcs.JJVCS{}, Focus: tt.f}
			s.refreshChangeIDFocus()
			if s.Focus != tt.f {
				t.Errorf("focus = %+v, want it untouched", s.Focus)
			}
		})
	}
}

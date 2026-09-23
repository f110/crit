package session

import (
	"fmt"
	"os"

	"github.com/tomasz-tomczyk/crit/internal/vcs"
)

// ChangeFocusLabel renders a change id the way jj's own log does: a short
// prefix, then the commit subject when there is one.
func ChangeFocusLabel(changeID, subject string) string {
	short := changeID
	if len(short) > 8 {
		short = short[:8]
	}
	if subject == "" {
		return short
	}
	return short + ": " + subject
}

// refreshChangeIDFocus re-points a change-id focus at the commits that back it
// now. Rewriting a JJ change moves both the change's own commit and, through
// the rebase, its parent — so a focus pinned by SHA goes stale after every
// round. The focus key is the change id and does not move, which is what keeps
// the round's comments attached across the swap.
//
// This is the range-focus counterpart of rereadFileContents: in working-tree
// mode the agent's new bytes land on disk, here they land in a rewritten
// commit. Both must run before captureRoundSnapshot.
//
// Deliberately not SetFocus. That path reloads the review file and re-captures
// a round snapshot, which is right for a user switching views and wrong in the
// middle of a round-complete that is already managing both.
//
// Called from the watcher goroutine with no locks held.
func (s *Session) refreshChangeIDFocus() {
	s.mu.RLock()
	current := s.Focus
	v := s.VCS
	repoRoot := s.RepoRoot
	s.mu.RUnlock()

	if current.Kind != FocusRange || current.VCSChangeID == "" || v == nil {
		return
	}
	next, err := changeFocusAtCurrentCommits(current, repoRoot)
	if err != nil {
		// An abandoned or divergent change must not tear down a review in
		// progress; showing the previous round's diff is the lesser harm.
		fmt.Fprintf(os.Stderr, "Warning: keeping the previous diff for change %s: %v\n", current.VCSChangeID, err)
		return
	}
	if next == current {
		return
	}
	files, baseRef, err := s.buildFilesForFocus(next, v, repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: rebuilding files for change %s: %v\n", current.VCSChangeID, err)
		return
	}

	s.mu.Lock()
	carryRoundStateLocked(s.Files, files)
	s.Focus = next
	s.Files = files
	s.BaseRef = baseRef
	s.mu.Unlock()
}

// changeFocusAtCurrentCommits returns f with BaseSHA, HeadSHA and Label
// re-resolved from the change id. Everything identifying the focus is left
// untouched.
func changeFocusAtCurrentCommits(f Focus, repoRoot string) (Focus, error) {
	head, err := vcs.ResolveJJChangeID(repoRoot, f.VCSChangeID)
	if err != nil {
		return f, err
	}
	base, err := vcs.JJChangeParentCommit(repoRoot, f.VCSChangeID)
	if err != nil {
		return f, err
	}
	f.HeadSHA = head
	f.BaseSHA = base
	f.Label = ChangeFocusLabel(f.VCSChangeID, vcs.JJCommitSubject(repoRoot, head))
	return f, nil
}

// carryRoundStateLocked moves the in-flight round's carry-forward inputs onto
// freshly built entries, matched by path. buildFilesForFocus returns entries
// with empty PreviousContent/PreviousComments; without this, carryForwardComments
// would find nothing to remap and every open comment would silently vanish.
//
// Comments are carried too: they hold anything authored in the window between
// SignalRoundComplete clearing them and the watcher getting here.
//
// Paths that dropped out of the change are not carried — restoreOrphanedComments
// later in round-complete rebuilds them from the review file.
func carryRoundStateLocked(old, next []*FileEntry) {
	if len(old) == 0 {
		return
	}
	prior := make(map[string]*FileEntry, len(old))
	for _, f := range old {
		prior[f.Path] = f
	}
	for _, f := range next {
		p, ok := prior[f.Path]
		if !ok {
			continue
		}
		f.PreviousContent = p.PreviousContent
		f.PreviousComments = p.PreviousComments
		f.Comments = p.Comments
	}
}

package focus

import (
	"strings"
	"testing"

	"github.com/tomasz-tomczyk/crit/internal/testutil"
	"github.com/tomasz-tomczyk/crit/internal/vcs"
)

func TestResolveFocus_ChangeIDIsExclusive(t *testing.T) {
	tests := []struct {
		name        string
		change      ChangeSpec
		rangeSpec   string
		remoteFiles bool
		wantErr     string
	}{
		{"with --pr", ChangeSpec{Forge: "github", Value: "1"}, "", false, "mutually exclusive"},
		{"with --mr", ChangeSpec{Forge: "gitlab", Value: "1"}, "", false, "mutually exclusive"},
		{"with --range", ChangeSpec{}, "a..b", false, "mutually exclusive"},
		{"with --remote", ChangeSpec{}, "", true, "--remote cannot be used with --change"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveFocus(tt.change, "knwmvumy", tt.rangeSpec, "", tt.remoteFiles, nil, "")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveFocus_ChangeIDRequiresJJ(t *testing.T) {
	_, err := ResolveFocus(ChangeSpec{}, "knwmvumy", "", "", false, &vcs.GitVCS{}, t.TempDir())
	if err == nil {
		t.Fatal("expected an error for a git repository")
	}
	if !strings.Contains(err.Error(), "requires a Jujutsu repository") {
		t.Errorf("error = %q, want it to mention Jujutsu", err)
	}
}

func TestResolveFocus_ChangeID(t *testing.T) {
	stack := testutil.InitJJStack(t)
	v := &vcs.JJVCS{}
	firstChange := stack.ChangeIDAt(t, "@--")
	wantHead := stack.CommitIDAt(t, "@--")
	wantBase := stack.CommitIDAt(t, "@---")

	f, err := ResolveFocus(ChangeSpec{}, firstChange, "", "", false, v, stack.Dir)
	if err != nil {
		t.Fatalf("ResolveFocus: %v", err)
	}
	if f == nil {
		t.Fatal("ResolveFocus returned nil focus")
	}
	if f.Kind != FocusRange {
		t.Errorf("Kind = %q, want %q", f.Kind, FocusRange)
	}
	if f.VCSChangeID != firstChange {
		t.Errorf("VCSChangeID = %q, want %q", f.VCSChangeID, firstChange)
	}
	if f.HeadSHA != wantHead {
		t.Errorf("HeadSHA = %q, want %q", f.HeadSHA, wantHead)
	}
	if f.BaseSHA != wantBase {
		t.Errorf("BaseSHA = %q, want %q (the change's parent)", f.BaseSHA, wantBase)
	}
	if f.DiffScope != DiffScopeLayer {
		t.Errorf("DiffScope = %q, want %q", f.DiffScope, DiffScopeLayer)
	}
	if want := firstChange[:8] + ": first"; f.Label != want {
		t.Errorf("Label = %q, want %q", f.Label, want)
	}
}

// TestResolveFocus_ChangeIDNormalizesPrefix guards the review-file identity:
// a focus keyed by the typed prefix would split one change across several
// review files depending on how much of the id the user typed.
func TestResolveFocus_ChangeIDNormalizesPrefix(t *testing.T) {
	stack := testutil.InitJJStack(t)
	v := &vcs.JJVCS{}
	firstChange := stack.ChangeIDAt(t, "@--")

	f, err := ResolveFocus(ChangeSpec{}, firstChange[:6], "", "", false, v, stack.Dir)
	if err != nil {
		t.Fatalf("ResolveFocus: %v", err)
	}
	if f.VCSChangeID != firstChange {
		t.Errorf("VCSChangeID = %q, want the full id %q", f.VCSChangeID, firstChange)
	}
}

// TestResolveFocus_ChangeIDAfterRewrite is the end-to-end form of the property
// the feature exists for: the same --change argument resolves to a moved commit
// while the focus identity stays put.
func TestResolveFocus_ChangeIDAfterRewrite(t *testing.T) {
	stack := testutil.InitJJStack(t)
	v := &vcs.JJVCS{}
	firstChange := stack.ChangeIDAt(t, "@--")

	before, err := ResolveFocus(ChangeSpec{}, firstChange, "", "", false, v, stack.Dir)
	if err != nil {
		t.Fatalf("ResolveFocus before rewrite: %v", err)
	}
	stack.Run(t, "describe", "-r", firstChange, "-m", "first rewritten")

	after, err := ResolveFocus(ChangeSpec{}, firstChange, "", "", false, v, stack.Dir)
	if err != nil {
		t.Fatalf("ResolveFocus after rewrite: %v", err)
	}
	if after.HeadSHA == before.HeadSHA {
		t.Error("HeadSHA did not move; the fixture did not rewrite anything")
	}
	if after.VCSChangeID != before.VCSChangeID {
		t.Errorf("VCSChangeID moved: %q -> %q", before.VCSChangeID, after.VCSChangeID)
	}
	if want := firstChange[:8] + ": first rewritten"; after.Label != want {
		t.Errorf("Label = %q, want %q", after.Label, want)
	}
}

func TestResolveFocus_ChangeIDRejectsMerge(t *testing.T) {
	stack := testutil.InitJJStack(t)
	v := &vcs.JJVCS{}
	stack.Run(t, "new", "@-", "@--", "-m", "merge")
	mergeChange := stack.ChangeIDAt(t, "@")

	_, err := ResolveFocus(ChangeSpec{}, mergeChange, "", "", false, v, stack.Dir)
	if err == nil {
		t.Fatal("expected an error for a merge commit")
	}
	if !strings.Contains(err.Error(), "merge commits") {
		t.Errorf("error = %q, want it to mention merge commits", err)
	}
}

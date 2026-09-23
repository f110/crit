package session

import "testing"

// TestFocusKeyFor_ChangeIDSurvivesRewrittenSHAs is the reason the change-id
// focus exists. Rewriting a JJ change moves its commit id and rebases every
// descendant onto new commit ids; a range key would change with them and hide
// the review's existing comments. The change-id key must not move.
func TestFocusKeyFor_ChangeIDSurvivesRewrittenSHAs(t *testing.T) {
	before := Focus{
		Kind:        FocusRange,
		VCSChangeID: "knwmvumyyonzwovwwvkswpxsloqzoyow",
		BaseSHA:     "755873608345accaf03a92e8cc3a2ebdc0eed696",
		HeadSHA:     "f83875d8bdcd1b2187b0babc6671f72d12168372",
		DiffScope:   DiffScopeLayer,
	}
	after := before
	after.BaseSHA = "07dbaa3214f3b40fbfa98961a3481228ec926315"
	after.HeadSHA = "44b5d4bbff3af62fd5506f48aefb8a263a5ed70e"

	if focusKeyFor(before) != focusKeyFor(after) {
		t.Fatalf("focus key moved with the SHAs: %q -> %q", focusKeyFor(before), focusKeyFor(after))
	}

	// A comment authored before the rewrite stays visible after it.
	c := StampWithFocus(Comment{}, before)
	if !visibleInFocus(c, after) {
		t.Error("comment authored before the rewrite is hidden after it")
	}
}

// TestFocusKeyFor_RangeKeyMovesWithSHAs documents the contrasting behaviour the
// change-id focus was added to work around.
func TestFocusKeyFor_RangeKeyMovesWithSHAs(t *testing.T) {
	before := Focus{Kind: FocusRange, BaseSHA: "aaa111", HeadSHA: "bbb222", DiffScope: DiffScopeLayer}
	after := Focus{Kind: FocusRange, BaseSHA: "ccc333", HeadSHA: "ddd444", DiffScope: DiffScopeLayer}

	if focusKeyFor(before) == focusKeyFor(after) {
		t.Fatal("range keys should differ once the SHAs differ")
	}
	c := StampWithFocus(Comment{}, before)
	if visibleInFocus(c, after) {
		t.Error("range-keyed comment unexpectedly visible under different SHAs")
	}
}

func TestFocusKeyArgs_VCSChangeID(t *testing.T) {
	sc := &CLIReviewConfig{Focus: &Focus{Kind: FocusRange, VCSChangeID: "knwmvumyyonz", BaseSHA: "aaa", HeadSHA: "bbb"}}
	got := FocusKeyArgs(sc)
	if len(got) != 1 || got[0] != "jjchange:knwmvumyyonz" {
		t.Errorf("got %v want [jjchange:knwmvumyyonz]", got)
	}
}

func TestInheritedScopeFrom_RoundTripsThroughAsFocus(t *testing.T) {
	f := Focus{
		Kind:         FocusRange,
		Forge:        "github",
		ChangeNumber: 42,
		VCSChangeID:  "knwmvumyyonz",
		BaseSHA:      "aaa111",
		HeadSHA:      "bbb222",
		DiffScope:    DiffScopeLayer,
	}
	got := InheritedScopeFrom(f, string(f.DiffScope)).AsFocus()

	if got.VCSChangeID != f.VCSChangeID {
		t.Errorf("VCSChangeID = %q, want %q", got.VCSChangeID, f.VCSChangeID)
	}
	if focusKeyFor(got) != focusKeyFor(f) {
		t.Errorf("focus key = %q, want %q", focusKeyFor(got), focusKeyFor(f))
	}
}

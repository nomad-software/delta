package git

import (
	"errors"
	"slices"
	"testing"
)

func TestMainBranch(t *testing.T) {
	for want, local := range map[string][]string{
		"main":   {"a", "master", "main"},
		"master": {"master", "topic"},
		"":       {"trunk", "topic"},
	} {
		if got := MainBranch(local); got != want {
			t.Errorf("%q gave %q", local, got)
		}
	}
}

// Finishing a branch merged upstream checks out main, pulls it with the merge,
// and deletes the branch; one not merged is kept, on main; and with no pull to
// be had the branch is kept too.
func TestFinishBranch(t *testing.T) {
	r := newRepo(t)
	bare := t.TempDir()
	git(t, bare, "init", "-q", "--bare", "-b", "main")
	git(t, r.Root, "remote", "add", "origin", bare)
	git(t, r.Root, "push", "-q", "-u", "origin", "main")
	git(t, r.Root, "switch", "-q", "-c", "topic")
	write(t, r.Root, "work", "done\n")
	git(t, r.Root, "add", "work")
	git(t, r.Root, "commit", "-q", "-m", "work")
	git(t, r.Root, "push", "-q", "origin", "topic")
	// Someone else merges it upstream.
	other := t.TempDir()
	git(t, other, "clone", "-q", bare, ".")
	git(t, other, "merge", "-q", "--ff-only", "origin/topic")
	git(t, other, "push", "-q", "origin", "main")

	if _, err := r.FinishBranch("topic", "main"); err != nil {
		t.Fatal(err)
	}
	local, _, _ := r.Branches()
	if s := status(t, r); s.Branch != "main" || slices.Contains(local, "topic") {
		t.Errorf("on %q with %q", s.Branch, local)
	}
	if got := git(t, r.Root, "log", "-1", "--format=%s"); got != "work\n" {
		t.Errorf("main is at %q, not pulled", got)
	}

	// Not merged: kept, on main.
	git(t, r.Root, "switch", "-q", "-c", "unmerged")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "mine")
	_, err := r.FinishBranch("unmerged", "main")
	local, _, _ = r.Branches()
	if !errors.Is(err, ErrBranchKept) || status(t, r).Branch != "main" || !slices.Contains(local, "unmerged") {
		t.Errorf("unmerged gave %v, on %q with %q", err, status(t, r).Branch, local)
	}

	// Nothing to pull from: kept, and not as one refused for not being merged.
	solo := newRepo(t)
	git(t, solo.Root, "switch", "-q", "-c", "topic")
	_, err = solo.FinishBranch("topic", "main")
	local, _, _ = solo.Branches()
	if err == nil || errors.Is(err, ErrBranchKept) || !slices.Contains(local, "topic") {
		t.Errorf("with no upstream gave %v, with %q", err, local)
	}
}

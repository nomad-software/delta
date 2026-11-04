package git

import (
	"errors"
	"strings"
	"testing"
)

// stashes returns the stashes, failing the test if they can't be read.
func stashes(t *testing.T, r *Repo) []Stash {
	t.Helper()
	s, err := r.Stashes()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Changes are stashed with their message, newest first, and put back with what
// was staged staged again; a pop drops the stash, an apply keeps it, and a drop
// deletes it.
func TestStash(t *testing.T) {
	r := newRepo(t)
	if got := stashes(t, r); len(got) != 0 {
		t.Fatalf("stashes to start with: %+v", got)
	}
	write(t, r.Root, "file", "changed\n")
	write(t, r.Root, "staged", "new\n")
	git(t, r.Root, "add", "staged")
	write(t, r.Root, "untracked", "loose\n")
	if _, err := r.StashChanges("first one", false); err != nil {
		t.Fatal(err)
	}
	// Untracked files stay unless asked for.
	if s := status(t, r); len(s.Staged) != 0 || len(s.Unstaged) != 1 || s.Unstaged[0].Path != "untracked" {
		t.Errorf("after stashing: %+v", s)
	}
	if _, err := r.StashChanges("", true); err != nil {
		t.Fatal(err)
	}
	if s := status(t, r); len(s.Staged)+len(s.Unstaged) != 0 {
		t.Errorf("after stashing untracked: %+v", s)
	}
	got := stashes(t, r)
	if len(got) != 2 || got[0].Ref != "stash@{0}" || got[1].Ref != "stash@{1}" || got[1].Subject != "On main: first one" || !strings.HasPrefix(got[0].Subject, "WIP on main:") {
		t.Fatalf("stashes %+v", got)
	}

	// Applying keeps the stash.
	if _, err := r.ApplyStash("stash@{1}", false, true); err != nil {
		t.Fatal(err)
	}
	s := status(t, r)
	if len(s.Staged) != 1 || s.Staged[0].Path != "staged" || len(s.Unstaged) != 1 || s.Unstaged[0].Path != "file" {
		t.Errorf("applied with its index: %+v", s)
	}
	if len(stashes(t, r)) != 2 {
		t.Error("apply dropped the stash")
	}
	git(t, r.Root, "reset", "-q", "--hard")
	git(t, r.Root, "clean", "-q", "-f")

	// Popping drops it.
	if _, err := r.ApplyStash("stash@{0}", true, true); err != nil {
		t.Fatal(err)
	}
	if got := stashes(t, r); len(got) != 1 || got[0].Subject != "On main: first one" {
		t.Errorf("after the pop: %+v", got)
	}
	if _, err := r.DropStash("stash@{0}"); err != nil {
		t.Fatal(err)
	}
	if got := stashes(t, r); len(got) != 0 {
		t.Errorf("after the drop: %+v", got)
	}
}

// A stash that clashes with what was committed since is applied with conflicts,
// and kept even by a pop; one whose staged changes can't be put back changes
// nothing, and applies with everything unstaged.
func TestStashConflicts(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "stashed\n")
	if _, err := r.StashChanges("mine", false); err != nil {
		t.Fatal(err)
	}
	write(t, r.Root, "file", "committed\n")
	git(t, r.Root, "commit", "-q", "-am", "two")
	_, err := r.ApplyStash("stash@{0}", true, false)
	if !errors.Is(err, ErrStashConflicts) || errors.Is(err, ErrStashIndex) {
		t.Fatalf("pop gave %v", err)
	}
	if s := status(t, r); len(s.Unstaged) != 1 || s.Unstaged[0].Code != Conflict {
		t.Errorf("after the conflict: %+v", s)
	}
	if len(stashes(t, r)) != 1 {
		t.Error("the pop that conflicted dropped the stash")
	}

	git(t, r.Root, "reset", "-q", "--hard")
	git(t, r.Root, "stash", "clear")
	write(t, r.Root, "file", "staged\n")
	git(t, r.Root, "add", "file")
	if _, err := r.StashChanges("", false); err != nil {
		t.Fatal(err)
	}
	write(t, r.Root, "file", "other\n")
	git(t, r.Root, "commit", "-q", "-am", "three")
	_, err = r.ApplyStash("stash@{0}", false, true)
	if !errors.Is(err, ErrStashIndex) || errors.Is(err, ErrStashConflicts) {
		t.Fatalf("apply with the index gave %v", err)
	}
	if s := status(t, r); len(s.Staged)+len(s.Unstaged) != 0 {
		t.Errorf("a refused apply changed %+v", s)
	}
	if _, err := r.ApplyStash("stash@{0}", false, false); !errors.Is(err, ErrStashConflicts) {
		t.Errorf("apply without the index gave %v", err)
	}
}

// A stash refused for what is in the way changes nothing and is neither kind of
// conflict: changes of the same file, or an untracked file it would replace.
func TestStashRefused(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "stashed\n")
	write(t, r.Root, "loose", "stashed\n")
	if _, err := r.StashChanges("", true); err != nil {
		t.Fatal(err)
	}
	write(t, r.Root, "file", "in the way\n")
	if _, err := r.ApplyStash("stash@{0}", true, true); err == nil || errors.Is(err, ErrStashConflicts) || errors.Is(err, ErrStashIndex) {
		t.Errorf("over changes gave %v", err)
	}
	git(t, r.Root, "checkout", "--", "file")
	write(t, r.Root, "loose", "in the way\n")
	if _, err := r.ApplyStash("stash@{0}", true, true); err == nil || errors.Is(err, ErrStashConflicts) || errors.Is(err, ErrStashIndex) {
		t.Errorf("over an untracked file gave %v", err)
	}
	if len(stashes(t, r)) != 1 {
		t.Error("a refused pop dropped the stash")
	}
}

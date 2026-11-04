package git

import (
	"strings"
	"testing"
)

// Uncommit takes the last commit back, its changes and any staged before left
// unstaged; the first commit goes back to none, its files untracked; a merge is
// refused.
func TestUncommit(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "second\n")
	write(t, r.Root, "added", "new\n")
	git(t, r.Root, "add", "file", "added")
	git(t, r.Root, "commit", "-q", "-m", "second")
	write(t, r.Root, "other", "staged\n")
	git(t, r.Root, "add", "other")
	if refused := r.ReadTip().UncommitRefused(); refused != "" {
		t.Fatal(refused)
	}
	if _, err := r.Uncommit(); err != nil {
		t.Fatal(err)
	}
	if got := headSubjects(t, r); strings.Join(got, " ") != "first" {
		t.Errorf("commits %q", got)
	}
	s := status(t, r)
	var unstaged []string
	for _, c := range s.Unstaged {
		unstaged = append(unstaged, string(c.Code)+" "+c.Path)
	}
	if len(s.Staged) != 0 || strings.Join(unstaged, ", ") != "M file, ? added, ? other" {
		t.Errorf("staged %+v, unstaged %q", s.Staged, unstaged)
	}

	// The first commit: none left, its files untracked.
	git(t, r.Root, "clean", "-q", "-f")
	git(t, r.Root, "checkout", "--", "file")
	if _, err := r.Uncommit(); err != nil {
		t.Fatal(err)
	}
	s = status(t, r)
	if len(s.Staged) != 0 || len(s.Unstaged) != 1 || s.Unstaged[0].Code != Untracked || s.Unstaged[0].Path != "file" || s.Branch != "main" {
		t.Errorf("after the first: %+v", s)
	}
	if refused := r.ReadTip().UncommitRefused(); refused == "" {
		t.Error("uncommit with no commits")
	}

	m := newRepo(t)
	git(t, m.Root, "switch", "-q", "-c", "topic")
	git(t, m.Root, "commit", "-q", "--allow-empty", "-m", "topic")
	git(t, m.Root, "switch", "-q", "main")
	git(t, m.Root, "commit", "-q", "--allow-empty", "-m", "main")
	git(t, m.Root, "merge", "-q", "--no-edit", "topic")
	if refused := m.ReadTip().UncommitRefused(); !strings.Contains(refused, "merge") {
		t.Errorf("a merge: %q", refused)
	}
	if _, err := m.Uncommit(); err == nil {
		t.Error("a merge was uncommitted")
	}
}

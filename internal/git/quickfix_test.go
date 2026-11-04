package git

import (
	"strings"
	"testing"
)

// headSubjects returns the subjects of the commits of HEAD, newest first.
func headSubjects(t *testing.T, r *Repo) []string {
	t.Helper()
	return strings.Fields(git(t, r.Root, "log", "--format=%s"))
}

// With changes not committed, a quick fix stages all of them, untracked files
// too, into the last commit, keeping its message.
func TestQuickFixAmends(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "second")
	write(t, r.Root, "file", "changed\n")
	write(t, r.Root, "staged", "new\n")
	git(t, r.Root, "add", "staged")
	write(t, r.Root, "loose", "untracked\n")
	q := r.PlanQuickFix(true)
	if !q.Amend || q.Last != "second" || q.Previous != "" || q.Refused != "" {
		t.Fatalf("plan %+v", q)
	}
	if _, err := r.RunQuickFix(q); err != nil {
		t.Fatal(err)
	}
	if s := status(t, r); len(s.Staged)+len(s.Unstaged) != 0 {
		t.Errorf("left %+v", s)
	}
	if got := headSubjects(t, r); strings.Join(got, " ") != "second first" {
		t.Errorf("commits %q", got)
	}
	if files := git(t, r.Root, "show", "--name-only", "--format=", "HEAD"); files != "file\nloose\nstaged\n" {
		t.Errorf("the last commit has %q", files)
	}
}

// With nothing to commit, a quick fix folds the last commit into the one
// before it, which keeps its message and author, onto a root commit too.
func TestQuickFixFolds(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "second\n")
	git(t, r.Root, "commit", "-q", "--author=Someone <someone@example.com>", "-am", "second\n\nIts body.")
	write(t, r.Root, "other", "third\n")
	git(t, r.Root, "add", "other")
	git(t, r.Root, "commit", "-q", "-m", "the fix")
	q := r.PlanQuickFix(false)
	if q.Amend || q.Last != "the fix" || q.Previous != "second" || q.Refused != "" {
		t.Fatalf("plan %+v", q)
	}
	if _, err := r.RunQuickFix(q); err != nil {
		t.Fatal(err)
	}
	if got := headSubjects(t, r); strings.Join(got, " ") != "second first" {
		t.Errorf("commits %q", got)
	}
	if got := git(t, r.Root, "log", "-1", "--format=%an|%B"); got != "Someone|second\n\nIts body.\n\n" {
		t.Errorf("message and author %q", got)
	}
	if files := git(t, r.Root, "show", "--name-only", "--format=", "HEAD"); files != "file\nother\n" {
		t.Errorf("the commit has %q", files)
	}
	if s := status(t, r); len(s.Staged)+len(s.Unstaged) != 0 {
		t.Errorf("left %+v", s)
	}

	// Onto the root: the result has no parent.
	q = r.PlanQuickFix(false)
	if q.Refused != "" {
		t.Fatalf("plan %+v", q)
	}
	if _, err := r.RunQuickFix(q); err != nil {
		t.Fatal(err)
	}
	if got := headSubjects(t, r); strings.Join(got, " ") != "first" {
		t.Errorf("commits %q", got)
	}
	if q := r.PlanQuickFix(false); q.Refused == "" {
		t.Errorf("a root commit could be folded: %+v", q)
	}
}

// A fold moves the branch only if it is still at the commit planned for.
func TestQuickFixFoldsOnlyWhatWasPlanned(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "second")
	q := r.PlanQuickFix(false)
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "since")
	if _, err := r.RunQuickFix(q); err == nil {
		t.Error("folded a commit no longer the last")
	}
	if got := headSubjects(t, r); strings.Join(got, " ") != "since second first" {
		t.Errorf("commits %q", got)
	}
}

// There is no quick fix without commits, nor a fold of or into a merge.
func TestQuickFixRefused(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	empty, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if q := empty.PlanQuickFix(true); q.Refused == "" {
		t.Error("a quick fix with no commits")
	}

	r := newRepo(t)
	git(t, r.Root, "switch", "-q", "-c", "topic")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "topic")
	git(t, r.Root, "switch", "-q", "main")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "main")
	git(t, r.Root, "merge", "-q", "--no-edit", "topic")
	if q := r.PlanQuickFix(false); !strings.Contains(q.Refused, "merge") {
		t.Errorf("fold of a merge: %+v", q)
	}
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "after")
	if q := r.PlanQuickFix(false); !strings.Contains(q.Refused, "merge") {
		t.Errorf("fold into a merge: %+v", q)
	}
	// Amending the merge is only a commit's amend.
	if q := r.PlanQuickFix(true); q.Refused != "" {
		t.Errorf("amend: %+v", q)
	}
}

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
	if refused := r.UncommitRefused(); refused != "" {
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
	if refused := r.UncommitRefused(); refused == "" {
		t.Error("uncommit with no commits")
	}

	m := newRepo(t)
	git(t, m.Root, "switch", "-q", "-c", "topic")
	git(t, m.Root, "commit", "-q", "--allow-empty", "-m", "topic")
	git(t, m.Root, "switch", "-q", "main")
	git(t, m.Root, "commit", "-q", "--allow-empty", "-m", "main")
	git(t, m.Root, "merge", "-q", "--no-edit", "topic")
	if refused := m.UncommitRefused(); !strings.Contains(refused, "merge") {
		t.Errorf("a merge: %q", refused)
	}
	if _, err := m.Uncommit(); err == nil {
		t.Error("a merge was uncommitted")
	}
}

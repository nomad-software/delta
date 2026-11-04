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
	q := r.ReadTip().QuickFix(true)
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
	q := r.ReadTip().QuickFix(false)
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
	q = r.ReadTip().QuickFix(false)
	if q.Refused != "" {
		t.Fatalf("plan %+v", q)
	}
	if _, err := r.RunQuickFix(q); err != nil {
		t.Fatal(err)
	}
	if got := headSubjects(t, r); strings.Join(got, " ") != "first" {
		t.Errorf("commits %q", got)
	}
	if q := r.ReadTip().QuickFix(false); q.Refused == "" {
		t.Errorf("a root commit could be folded: %+v", q)
	}
}

// A fold moves the branch only if it is still at the commit planned for.
func TestQuickFixFoldsOnlyWhatWasPlanned(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "second")
	q := r.ReadTip().QuickFix(false)
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
	if q := empty.ReadTip().QuickFix(true); q.Refused == "" {
		t.Error("a quick fix with no commits")
	}

	r := newRepo(t)
	git(t, r.Root, "switch", "-q", "-c", "topic")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "topic")
	git(t, r.Root, "switch", "-q", "main")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "main")
	git(t, r.Root, "merge", "-q", "--no-edit", "topic")
	if q := r.ReadTip().QuickFix(false); !strings.Contains(q.Refused, "merge") {
		t.Errorf("fold of a merge: %+v", q)
	}
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "after")
	if q := r.ReadTip().QuickFix(false); !strings.Contains(q.Refused, "merge") {
		t.Errorf("fold into a merge: %+v", q)
	}
	// Amending the merge is only a commit's amend.
	if q := r.ReadTip().QuickFix(true); q.Refused != "" {
		t.Errorf("amend: %+v", q)
	}
}

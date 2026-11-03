package git

import (
	"slices"
	"strings"
	"testing"
)

// commitFile commits a file with the text and message.
func commitFile(t *testing.T, r *Repo, name, text, message string) {
	t.Helper()
	write(t, r.Root, name, text)
	git(t, r.Root, "add", name)
	git(t, r.Root, "commit", "-q", "-m", message)
}

// subjects returns the subjects of the commits from HEAD down, oldest first.
func subjects(t *testing.T, r *Repo) []string {
	t.Helper()
	return strings.Split(strings.TrimSpace(git(t, r.Root, "log", "--reverse", "--format=%s")), "\n")
}

// rebaseSteps returns the commits from the one with the subject up to HEAD, each
// to be picked, and their base.
func rebaseSteps(t *testing.T, r *Repo, subject string) ([]Step, string) {
	t.Helper()
	hash := strings.TrimSpace(git(t, r.Root, "log", "--format=%H", "--grep=^"+subject+"$"))
	commits, base, err := r.RebaseCommits(hash)
	if err != nil {
		t.Fatal(err)
	}
	steps := make([]Step, len(commits))
	for i, c := range commits {
		steps[i] = Step{Commit: c, Action: Pick}
	}
	return steps, base
}

func TestRebaseCommits(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a", "a\n", "second\n\nWith a body\nof two lines.")
	commitFile(t, r, "b", "b\n", "third")
	steps, base := rebaseSteps(t, r, "second")
	if len(steps) != 2 || steps[0].Commit.Subject != "second" || steps[1].Commit.Subject != "third" {
		t.Fatalf("steps %+v", steps)
	}
	if want := "second\n\nWith a body\nof two lines."; steps[0].Commit.Message != want {
		t.Errorf("message %q", steps[0].Commit.Message)
	}
	if first := strings.TrimSpace(git(t, r.Root, "rev-parse", "HEAD~2")); base != first {
		t.Errorf("base %q, want %q", base, first)
	}
	// From the first commit, there is nothing to go on top of.
	steps, base = rebaseSteps(t, r, "first")
	if len(steps) != 3 || base != "" {
		t.Errorf("from the root: %d steps on %q", len(steps), base)
	}
}

func TestRebaseCommitsRefusesMerges(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "switch", "-q", "-c", "topic")
	commitFile(t, r, "side", "side\n", "side")
	git(t, r.Root, "switch", "-q", "main")
	commitFile(t, r, "main", "main\n", "main")
	git(t, r.Root, "merge", "-q", "--no-edit", "topic")
	hash := strings.TrimSpace(git(t, r.Root, "rev-parse", "HEAD~1"))
	if _, _, err := r.RebaseCommits(hash); err == nil || !strings.Contains(err.Error(), "merge") {
		t.Errorf("across a merge: %v", err)
	}
	if _, _, err := r.RebaseCommits("HEAD"); err == nil || !strings.Contains(err.Error(), "merge") {
		t.Errorf("from a merge: %v", err)
	}
	git(t, r.Root, "switch", "-q", "topic")
	if _, _, err := r.RebaseCommits("main"); err == nil {
		t.Error("from a commit not in the branch")
	}
}

func TestRebaseActions(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a", "a\n", "a")
	commitFile(t, r, "b", "b\n", "b")
	commitFile(t, r, "c", "c\n", "c")
	commitFile(t, r, "d", "d\n", "d")
	commitFile(t, r, "e", "e\n", "e")
	steps, base := rebaseSteps(t, r, "a")
	// Reorder, reword with quotes and blank lines, squash, fix up and drop.
	steps[0].Action, steps[0].Message = Reword, "it's \"a\"\n\n$HOME `b`\n"
	steps[1].Action = Squash
	steps[2], steps[3] = steps[3], steps[2]
	steps[3].Action = Fixup
	steps[4].Action = Drop
	if _, err := r.Rebase(base, steps); err != nil {
		t.Fatal(err)
	}
	if r.Rebasing() {
		t.Fatal("still rebasing")
	}
	if got := subjects(t, r); !slices.Equal(got, []string{"first", `it's "a"`, "d"}) {
		t.Errorf("subjects %q", got)
	}
	message := strings.TrimSpace(git(t, r.Root, "log", "-1", "--format=%B", "HEAD~1"))
	if want := "it's \"a\"\n\n$HOME `b`\n\nb"; message != want {
		t.Errorf("squashed message %q, want %q", message, want)
	}
	if message := strings.TrimSpace(git(t, r.Root, "log", "-1", "--format=%B")); message != "d" {
		t.Errorf("fixed up message %q", message)
	}
	if files := strings.Fields(git(t, r.Root, "ls-files")); !slices.Equal(files, []string{"a", "b", "c", "d", "file"}) {
		t.Errorf("files %q", files)
	}
}

func TestRebaseStopsToEdit(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a", "a\n", "a")
	commitFile(t, r, "b", "b\n", "b")
	steps, base := rebaseSteps(t, r, "a")
	steps[0].Action = Edit
	if _, err := r.Rebase(base, steps); err != nil {
		t.Fatal(err)
	}
	s := status(t, r)
	if !s.Rebasing || !s.Rebase.Amending || s.Rebase.Step != 1 || s.Rebase.Steps != 2 {
		t.Fatalf("status %+v", s)
	}
	// What is staged goes into the commit being edited.
	write(t, r.Root, "a", "changed\n")
	git(t, r.Root, "add", "a")
	if _, err := r.ContinueRebase(); err != nil {
		t.Fatal(err)
	}
	if r.Rebasing() {
		t.Fatal("still rebasing")
	}
	if got := subjects(t, r); !slices.Equal(got, []string{"first", "a", "b"}) {
		t.Errorf("subjects %q", got)
	}
	if got := git(t, r.Root, "show", "HEAD~1:a"); got != "changed\n" {
		t.Errorf("a is %q", got)
	}
}

func TestRebaseConflict(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "file", "two\n", "two")
	commitFile(t, r, "file", "three\n", "three")
	steps, base := rebaseSteps(t, r, "two")
	steps[0], steps[1] = steps[1], steps[0]
	if _, err := r.Rebase(base, steps); err == nil {
		t.Fatal("no conflict")
	}
	s := status(t, r)
	if !s.Rebasing || s.Rebase.Amending || len(s.Unstaged) != 1 || s.Unstaged[0].Code != Conflict {
		t.Fatalf("status %+v", s)
	}
	if _, err := r.AbortRebase(); err != nil {
		t.Fatal(err)
	}
	if r.Rebasing() {
		t.Error("still rebasing")
	}
	if got := subjects(t, r); !slices.Equal(got, []string{"first", "two", "three"}) {
		t.Errorf("subjects %q", got)
	}

	// Resolved, it carries on with the commit's own message, and skipping
	// leaves a commit out.
	if _, err := r.Rebase(base, steps); err == nil {
		t.Fatal("no conflict")
	}
	write(t, r.Root, "file", "three\n")
	git(t, r.Root, "add", "file")
	if _, err := r.ContinueRebase(); err == nil {
		t.Fatal("no second conflict")
	}
	if _, err := r.SkipRebase(); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, r); !slices.Equal(got, []string{"first", "three"}) {
		t.Errorf("subjects %q", got)
	}
}

func TestTodo(t *testing.T) {
	steps := []Step{
		{Commit: Commit{Hash: "1", Subject: "one"}, Action: Fixup},
		{Commit: Commit{Hash: "2", Subject: "two"}, Action: Reword, Message: "a 'b'\r\n\nc\n"},
	}
	want := "fixup 1 one\npick 2 two\nexec printf '%s\\n' 'a '\\''b'\\''' '' 'c' | git commit --amend --allow-empty --quiet --file=-\n"
	if got := todo(steps); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

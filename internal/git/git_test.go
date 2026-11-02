package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newRepo returns a repository with one commit, of a file named "file",
// isolated from the configuration of whoever runs the tests.
func newRepo(t *testing.T) *Repo {
	t.Helper()
	isolate(t)
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	write(t, dir, "file", "one\n")
	git(t, dir, "add", "file")
	git(t, dir, "commit", "-m", "first")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, who := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+who+"_NAME", "Test")
		t.Setenv("GIT_"+who+"_EMAIL", "test@example.com")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func status(t *testing.T, r *Repo) Status {
	t.Helper()
	s, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenFromSubfolderAndOutside(t *testing.T) {
	r := newRepo(t)
	sub := filepath.Join(r.Root, "sub")
	os.Mkdir(sub, 0o755)
	if got, err := Open(sub); err != nil || got.Root != r.Root {
		t.Errorf("Open(sub) = %v, %v", got, err)
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("a folder outside a repository was opened")
	}
}

func TestParseStatus(t *testing.T) {
	out := strings.Join([]string{
		"# branch.oid abc",
		"# branch.head topic",
		"# branch.upstream origin/topic",
		"# branch.ab +2 -3",
		"1 MM N... 100644 100644 100644 aaa bbb both spaces.txt",
		"1 .D N... 100644 100644 000000 aaa aaa gone",
		"2 R. N... 100644 100644 100644 aaa bbb R100 new name",
		"old name",
		"u UU N... 100644 100644 100644 100644 a b c fought over",
		"? new file",
		"! ignored",
		"",
	}, "\x00")
	s, err := parseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "topic" || s.Upstream != "origin/topic" || s.Ahead != 2 || s.Behind != 3 {
		t.Errorf("header read as %+v", s)
	}
	wantStaged := []Change{{Path: "both spaces.txt", Code: 'M'}, {Path: "new name", OldPath: "old name", Code: 'R'}}
	if !slices.Equal(s.Staged, wantStaged) {
		t.Errorf("staged %+v", s.Staged)
	}
	wantUnstaged := []Change{
		{Path: "both spaces.txt", Code: 'M'}, {Path: "gone", Code: 'D'},
		{Path: "fought over", Code: Conflict}, {Path: "new file", Code: Untracked},
	}
	if !slices.Equal(s.Unstaged, wantUnstaged) {
		t.Errorf("unstaged %+v", s.Unstaged)
	}
	if _, err := parseStatus("1 short\x00"); err == nil {
		t.Error("a malformed line was accepted")
	}
}

func TestStageUnstageCommit(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "two\n")
	write(t, r.Root, "-odd [name]*", "new\n")
	s := status(t, r)
	if len(s.Unstaged) != 2 || len(s.Staged) != 0 {
		t.Fatalf("status %+v", s)
	}
	if err := r.Stage(s.Unstaged); err != nil {
		t.Fatal(err)
	}
	s = status(t, r)
	if len(s.Unstaged) != 0 || len(s.Staged) != 2 {
		t.Fatalf("after staging %+v", s)
	}
	if err := r.Unstage(s.Staged[:1]); err != nil {
		t.Fatal(err)
	}
	s = status(t, r)
	if len(s.Unstaged) != 1 || len(s.Staged) != 1 {
		t.Fatalf("after unstaging one %+v", s)
	}
	if _, err := r.Commit("a message\n\n# kept, not a comment"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, r.Root, "log", "-1", "--format=%B"); !strings.Contains(got, "# kept") {
		t.Errorf("message came out as %q", got)
	}
}

func TestStageDeletedAndUnstageRename(t *testing.T) {
	r := newRepo(t)
	if err := os.Rename(filepath.Join(r.Root, "file"), filepath.Join(r.Root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := r.Stage(status(t, r).Unstaged); err != nil {
		t.Fatal(err)
	}
	s := status(t, r)
	if len(s.Staged) != 1 || s.Staged[0].Code != 'R' || s.Staged[0].OldPath != "file" {
		t.Fatalf("rename staged as %+v", s.Staged)
	}
	if d, err := r.Diff(s.Staged[0], true); err != nil || !strings.Contains(d, "rename to moved") {
		t.Errorf("diff of a rename: %q, %v", d, err)
	}
	if err := r.Unstage(s.Staged); err != nil {
		t.Fatal(err)
	}
	if s = status(t, r); len(s.Staged) != 0 {
		t.Errorf("rename still staged: %+v", s.Staged)
	}
}

func TestUnstageOnUnbornBranch(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	write(t, dir, "a", "a\n")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := status(t, r)
	if s.Branch != "main" {
		t.Errorf("branch %q", s.Branch)
	}
	if err := r.Stage(s.Unstaged); err != nil {
		t.Fatal(err)
	}
	if err := r.Unstage(status(t, r).Staged); err != nil {
		t.Fatal(err)
	}
	if s = status(t, r); len(s.Staged) != 0 || len(s.Unstaged) != 1 {
		t.Errorf("status %+v", s)
	}
}

func TestDiffs(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "two\n")
	write(t, r.Root, "untracked", "new\n")
	d, err := r.Diff(Change{Path: "file", Code: 'M'}, false)
	if err != nil || !strings.Contains(d, "-one") || !strings.Contains(d, "+two") {
		t.Errorf("unstaged diff %q, %v", d, err)
	}
	d, err = r.Diff(Change{Path: "untracked", Code: Untracked}, false)
	if err != nil || !strings.Contains(d, "+new") {
		t.Errorf("untracked diff %q, %v", d, err)
	}
	if err := r.Stage([]Change{{Path: "file"}}); err != nil {
		t.Fatal(err)
	}
	d, err = r.Diff(Change{Path: "file", Code: 'M'}, true)
	if err != nil || !strings.Contains(d, "+two") {
		t.Errorf("staged diff %q, %v", d, err)
	}
}

func TestDiffIsCut(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "big", strings.Repeat("a line of text\n", MaxDiff/10))
	d, err := r.Diff(Change{Path: "big", Code: Untracked}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d) > MaxDiff+100 || !strings.Contains(d, "[The rest is not shown") {
		t.Errorf("%d bytes, ending %q", len(d), d[len(d)-60:])
	}
}

func TestBranches(t *testing.T) {
	r := newRepo(t)
	if !r.ValidBranchName("topic/one") || r.ValidBranchName("-bad") || r.ValidBranchName("a b") {
		t.Error("branch names misjudged")
	}
	if _, err := r.CreateBranch("topic", "", true); err != nil {
		t.Fatal(err)
	}
	if b := status(t, r).Branch; b != "topic" {
		t.Errorf("on %q", b)
	}
	if _, err := r.CreateBranch("other", "main", false); err != nil {
		t.Fatal(err)
	}
	if b := status(t, r).Branch; b != "topic" {
		t.Errorf("moved to %q", b)
	}
	if _, err := r.RenameBranch("other", "renamed"); err != nil {
		t.Fatal(err)
	}
	local, _, err := r.Branches()
	if err != nil || !slices.Equal(local, []string{"main", "renamed", "topic"}) {
		t.Errorf("branches %v, %v", local, err)
	}
	// A branch with work of its own is only deleted by force.
	write(t, r.Root, "file", "two\n")
	if _, err := r.Commit("nothing staged"); err == nil {
		t.Error("an empty commit was made")
	}
	if err := r.Stage([]Change{{Path: "file"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DeleteBranch("topic", false); err == nil {
		t.Error("an unmerged branch was deleted")
	}
	if _, err := r.DeleteBranch("topic", true); err != nil {
		t.Error(err)
	}
}

// TestMergeConflict goes through a merge that fails, and shows that unstaging
// during it does not abandon it.
func TestMergeConflict(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "switch", "-c", "topic")
	write(t, r.Root, "file", "theirs\n")
	write(t, r.Root, "clean", "clean\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "theirs")
	git(t, r.Root, "switch", "main")
	write(t, r.Root, "file", "ours\n")
	git(t, r.Root, "commit", "-am", "ours")

	_, err := r.Merge("topic")
	if err == nil || !strings.Contains(err.Error(), "CONFLICT") {
		t.Fatalf("merge said %v", err)
	}
	s := status(t, r)
	if !s.Merging || len(s.Unstaged) != 1 || s.Unstaged[0].Code != Conflict {
		t.Fatalf("status %+v", s)
	}
	if !strings.Contains(r.MergeMessage(), "Merge branch 'topic'") {
		t.Errorf("merge message %q", r.MergeMessage())
	}
	if err := r.Unstage(s.Staged); err != nil {
		t.Fatal(err)
	}
	if !status(t, r).Merging {
		t.Error("unstaging abandoned the merge")
	}
	if _, err := r.AbortMerge(); err != nil {
		t.Fatal(err)
	}
	if status(t, r).Merging {
		t.Error("the merge was not aborted")
	}
}

func TestLogAndDetail(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "mv", "file", "moved")
	write(t, r.Root, "added", "added\n")
	git(t, r.Root, "add", "added")
	git(t, r.Root, "commit", "-m", "second\n\nthe body")

	commits, err := r.Log(10, 0)
	if err != nil || len(commits) != 2 {
		t.Fatalf("log %+v, %v", commits, err)
	}
	second, first := commits[0], commits[1]
	if second.Subject != "second" || second.Author != "Test" || len(second.Parents) != 1 || second.Parents[0] != first.Hash {
		t.Errorf("second %+v", second)
	}
	if !strings.Contains(second.Refs, "main") || second.Date.IsZero() {
		t.Errorf("second %+v", second)
	}
	if page, _ := r.Log(1, 1); len(page) != 1 || page[0].Hash != first.Hash {
		t.Errorf("second page %+v", page)
	}

	changes, text, err := r.CommitDetail(second)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: "added", Code: 'A'}, {Path: "moved", OldPath: "file", Code: 'R'}}
	if !slices.Equal(changes, want) {
		t.Errorf("files %+v", changes)
	}
	for _, part := range []string{"commit " + second.Hash, "Author:", "the body", "+added", "rename to moved"} {
		if !strings.Contains(text, part) {
			t.Errorf("no %q in %q", part, text)
		}
	}

	// The first commit has no parent to compare with.
	changes, text, err = r.CommitDetail(first)
	if err != nil || !slices.Equal(changes, []Change{{Path: "file", Code: 'A'}}) || !strings.Contains(text, "+one") {
		t.Errorf("first: %+v, %q, %v", changes, text, err)
	}
}

func TestDetailOfMergeIsAgainstFirstParent(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "switch", "-c", "topic")
	write(t, r.Root, "theirs", "theirs\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "theirs")
	git(t, r.Root, "switch", "main")
	write(t, r.Root, "ours", "ours\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "ours")
	git(t, r.Root, "merge", "--no-edit", "topic")
	commits, err := r.Log(1, 0)
	if err != nil || len(commits[0].Parents) != 2 {
		t.Fatalf("log %+v, %v", commits, err)
	}
	changes, text, err := r.CommitDetail(commits[0])
	if err != nil || !slices.Equal(changes, []Change{{Path: "theirs", Code: 'A'}}) || !strings.Contains(text, "+theirs") {
		t.Errorf("merge: %+v, %q, %v", changes, text, err)
	}
}

func TestUncommittedDetail(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "unstaged\n")
	write(t, r.Root, "staged", "staged\n")
	git(t, r.Root, "add", "staged")
	write(t, r.Root, "untracked", "untracked\n")
	changes, text, err := r.CommitDetail(UncommittedCommit("abc"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: "file", Code: 'M'}, {Path: "staged", Code: 'A'}, {Path: "untracked", Code: Untracked}}
	if !slices.Equal(changes, want) {
		t.Errorf("files %+v", changes)
	}
	for _, part := range []string{"+unstaged", "+staged", "+untracked", "b/file", "b/staged", "b/untracked"} {
		if !strings.Contains(text, part) {
			t.Errorf("no %q in %q", part, text)
		}
	}
	if strings.Count(text, "diff --git") != 3 {
		t.Errorf("not one diff to a file: %q", text)
	}
}

func TestUncommittedDetailOfUnbornBranch(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	write(t, dir, "staged", "staged\n")
	git(t, dir, "add", "staged")
	write(t, dir, "untracked", "untracked\n")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, text, err := r.CommitDetail(UncommittedCommit(""))
	if err != nil || len(changes) != 2 || !strings.Contains(text, "+staged") || !strings.Contains(text, "+untracked") {
		t.Errorf("%+v, %q, %v", changes, text, err)
	}
}

func TestLogOfUnbornBranch(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if commits, err := r.Log(10, 0); err != nil || len(commits) != 0 {
		t.Errorf("log %+v, %v", commits, err)
	}
}

func TestParseOddOutput(t *testing.T) {
	if _, err := parseLog("too\x00few\n"); err == nil {
		t.Error("a malformed log line was accepted")
	}
	if _, err := parseNameStatus("R100\x00only\x00"); err == nil {
		t.Error("a rename with no new path was accepted")
	}
	got, err := parseNameStatus("M\x00odd name\x00R90\x00old\x00new\x00")
	want := []Change{{Path: "odd name", Code: 'M'}, {Path: "new", OldPath: "old", Code: 'R'}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestRemotes(t *testing.T) {
	r := newRepo(t)
	bare := t.TempDir()
	git(t, bare, "init", "--bare", "-b", "main")
	if _, err := r.AddRemote("upstream", bare); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenameRemote("upstream", "origin"); err != nil {
		t.Fatal(err)
	}
	if u, err := r.RemoteURL("origin"); err != nil || u != bare {
		t.Errorf("url %q, %v", u, err)
	}

	// The first push makes the upstream.
	if _, err := r.Push("origin", false); err != nil {
		t.Fatal(err)
	}
	s := status(t, r)
	if s.Upstream != "origin/main" {
		t.Errorf("upstream %q", s.Upstream)
	}
	if _, remote, _ := r.Branches(); !slices.Equal(remote, []string{"origin/main"}) {
		t.Errorf("remote branches %v", remote)
	}

	// A rewritten commit is only pushed by force.
	write(t, r.Root, "file", "two\n")
	git(t, r.Root, "commit", "-a", "--amend", "-m", "rewritten")
	if _, err := r.Push("", false); err == nil {
		t.Error("a rewrite was pushed without force")
	}
	if _, err := r.Push("", true); err != nil {
		t.Errorf("force push: %v", err)
	}
	if s = status(t, r); s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("after force push %+v", s)
	}

	if _, err := r.Fetch(); err != nil {
		t.Error(err)
	}
	if _, err := r.Pull(); err != nil {
		t.Error(err)
	}

	// A checkout of a remote branch makes a local branch that tracks it.
	git(t, r.Root, "branch", "side")
	git(t, r.Root, "push", "origin", "side")
	git(t, r.Root, "branch", "-D", "side")
	if _, err := r.CheckoutRemote("origin/side"); err != nil {
		t.Fatal(err)
	}
	if s = status(t, r); s.Branch != "side" || s.Upstream != "origin/side" {
		t.Errorf("after remote checkout %+v", s)
	}

	if _, err := r.SetRemoteURL("origin", "/nowhere"); err != nil {
		t.Error(err)
	}
	if _, err := r.RemoveRemote("origin"); err != nil {
		t.Error(err)
	}
	if rs, _ := r.Remotes(); len(rs) != 0 {
		t.Errorf("remotes %v", rs)
	}
}

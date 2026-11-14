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

	changes, err := r.CommitFiles(second)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: "added", Code: 'A'}, {Path: "moved", OldPath: "file", Code: 'R'}}
	if !slices.Equal(changes, want) {
		t.Errorf("files %+v", changes)
	}
	// Each file's diff is its own; a rename is one with the file it came from.
	if text := fileDiff(t, r, second, changes[0]); !strings.Contains(text, "+added") || strings.Contains(text, "moved") {
		t.Errorf("added: %q", text)
	}
	if text := fileDiff(t, r, second, changes[1]); !strings.Contains(text, "rename to moved") || strings.Contains(text, "added") {
		t.Errorf("moved: %q", text)
	}

	// The first commit has no parent to compare with.
	changes, err = r.CommitFiles(first)
	if err != nil || !slices.Equal(changes, []Change{{Path: "file", Code: 'A'}}) {
		t.Fatalf("first: %+v, %v", changes, err)
	}
	if text := fileDiff(t, r, first, changes[0]); !strings.Contains(text, "+one") {
		t.Errorf("first: %q", text)
	}
}

// fileDiff returns the diff of one of a commit's files.
func fileDiff(t *testing.T, r *Repo, c Commit, f Change) string {
	t.Helper()
	text, err := r.CommitFileDiff(c, f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(text, "diff --") != 1 {
		t.Errorf("not the diff of one file: %q", text)
	}
	return text
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
	changes, err := r.CommitFiles(commits[0])
	if err != nil || !slices.Equal(changes, []Change{{Path: "theirs", Code: 'A'}}) {
		t.Fatalf("merge: %+v, %v", changes, err)
	}
	if text := fileDiff(t, r, commits[0], changes[0]); !strings.Contains(text, "+theirs") {
		t.Errorf("merge: %q", text)
	}
}

func TestUncommittedDetail(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "unstaged\n")
	write(t, r.Root, "staged", "staged\n")
	git(t, r.Root, "add", "staged")
	write(t, r.Root, "untracked", "untracked\n")
	uncommitted := UncommittedCommit("abc")
	changes, err := r.CommitFiles(uncommitted)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: "file", Code: 'M'}, {Path: "staged", Code: 'A'}, {Path: "untracked", Code: Untracked}}
	if !slices.Equal(changes, want) {
		t.Fatalf("files %+v", changes)
	}
	// Staged or not, each is against HEAD.
	for i, part := range []string{"+unstaged", "+staged", "+untracked"} {
		if text := fileDiff(t, r, uncommitted, changes[i]); !strings.Contains(text, part) || !strings.Contains(text, "b/"+changes[i].Path) {
			t.Errorf("%s: %q", changes[i].Path, text)
		}
	}
}

// A file's diff is cut off as Diff says, here well short of MaxDiff, as its last
// line is long.
func TestCommitFileDiffIsCut(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "short\n"+strings.Repeat("x", MaxDiff+1)+"\n")
	text, err := r.CommitFileDiff(UncommittedCommit(""), Change{Path: "file", Code: 'M'})
	if err != nil || !strings.HasSuffix(text, cutNote()) || len(text) > MaxDiff+len(cutNote()) {
		t.Errorf("%v, %d bytes", err, len(text))
	}
}

// TestStageAfterStagedRename checks that a file renamed in the index and then
// changed can be staged, as its old path belongs only to the staged side.
func TestStageAfterStagedRename(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "old", "one\ntwo\nthree\nfour\nfive\n")
	if err := r.Stage([]Change{{Path: "old"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit("old"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.run("mv", "old", "new"); err != nil {
		t.Fatal(err)
	}
	write(t, r.Root, "new", "one\ntwo\nthree\nfour\nfive\nsix\n")
	s, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	wantStaged := []Change{{Path: "new", OldPath: "old", Code: 'R'}}
	wantUnstaged := []Change{{Path: "new", Code: 'M'}}
	if !slices.Equal(s.Staged, wantStaged) || !slices.Equal(s.Unstaged, wantUnstaged) {
		t.Fatalf("staged %+v, unstaged %+v", s.Staged, s.Unstaged)
	}
	if d, err := r.Diff(s.Unstaged[0], false); err != nil || !strings.Contains(d, "+six") {
		t.Errorf("diff %q, %v", d, err)
	}
	if err := r.Stage(s.Unstaged); err != nil {
		t.Fatal(err)
	}
	if s, err = r.Status(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Staged, wantStaged) || len(s.Unstaged) != 0 {
		t.Errorf("staged %+v, unstaged %+v", s.Staged, s.Unstaged)
	}
}

// TestOldPathBelongsToOneSide checks that a status line's old path only goes on
// the side of the index that renamed or copied the file.
func TestOldPathBelongsToOneSide(t *testing.T) {
	const hashes = "N... 100644 100644 100644 aaaa bbbb"
	moved := func(code byte) Change { return Change{Path: "new", OldPath: "old", Code: code} }
	plain := func(code byte) Change { return Change{Path: "new", Code: code} }
	for _, test := range []struct {
		xy               string
		staged, unstaged []Change
	}{
		{"R.", []Change{moved('R')}, nil},
		{"RM", []Change{moved('R')}, []Change{plain('M')}},
		{"RD", []Change{moved('R')}, []Change{plain('D')}},
		{"C.", []Change{moved('C')}, nil},
		{"CM", []Change{moved('C')}, []Change{plain('M')}},
		{".R", nil, []Change{moved('R')}},
		{".C", nil, []Change{moved('C')}},
		{"MR", []Change{plain('M')}, []Change{moved('R')}},
		{"AR", []Change{plain('A')}, []Change{moved('R')}},
	} {
		s, err := parseStatus("2 " + test.xy + " " + hashes + " R100 new\x00old\x00")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(s.Staged, test.staged) || !slices.Equal(s.Unstaged, test.unstaged) {
			t.Errorf("%s: staged %+v, unstaged %+v", test.xy, s.Staged, s.Unstaged)
		}
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
	uncommitted := UncommittedCommit("")
	changes, err := r.CommitFiles(uncommitted)
	if err != nil || len(changes) != 2 {
		t.Fatalf("%+v, %v", changes, err)
	}
	if text := fileDiff(t, r, uncommitted, changes[0]); !strings.Contains(text, "+staged") {
		t.Errorf("staged: %q", text)
	}
	if text := fileDiff(t, r, uncommitted, changes[1]); !strings.Contains(text, "+untracked") {
		t.Errorf("untracked: %q", text)
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

func TestDiscardChanges(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, ".gitignore", "*.log\n")
	write(t, r.Root, "kept", "kept\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-q", "-m", "second")
	write(t, r.Root, "file", "changed\n")
	write(t, r.Root, "staged", "staged\n")
	git(t, r.Root, "add", "staged")
	git(t, r.Root, "rm", "-q", "kept")
	write(t, r.Root, "untracked", "untracked\n")
	if err := os.MkdirAll(filepath.Join(r.Root, "dir", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, r.Root, "dir/deeper/new", "new\n")
	write(t, r.Root, "build.log", "ignored\n")
	if _, err := r.DiscardChanges(); err != nil {
		t.Fatal(err)
	}
	if s := status(t, r); len(s.Staged)+len(s.Unstaged) != 0 {
		t.Errorf("left %+v", s)
	}
	if got := git(t, r.Root, "show", "HEAD:file"); got != "one\n" {
		t.Errorf("file is %q", got)
	}
	for _, gone := range []string{"untracked", "dir", "staged"} {
		if _, err := os.Stat(filepath.Join(r.Root, gone)); !os.IsNotExist(err) {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, there := range []string{"kept", "build.log"} {
		if _, err := os.Stat(filepath.Join(r.Root, there)); err != nil {
			t.Errorf("%s is gone", there)
		}
	}
	if got := strings.TrimSpace(git(t, r.Root, "log", "-1", "--format=%s")); got != "second" {
		t.Errorf("HEAD is %q", got)
	}
}

func TestDiscardChangesAbandonsAMerge(t *testing.T) {
	r := newRepo(t)
	git(t, r.Root, "switch", "-q", "-c", "topic")
	write(t, r.Root, "file", "theirs\n")
	git(t, r.Root, "commit", "-qam", "theirs")
	git(t, r.Root, "switch", "-q", "main")
	write(t, r.Root, "file", "ours\n")
	git(t, r.Root, "commit", "-qam", "ours")
	if _, err := r.Merge("topic"); err == nil {
		t.Fatal("no conflict")
	}
	if _, err := r.DiscardChanges(); err != nil {
		t.Fatal(err)
	}
	if s := status(t, r); s.Merging || len(s.Unstaged)+len(s.Staged) != 0 {
		t.Errorf("status %+v", s)
	}
}

func TestDiscardChangesOnUnbornBranch(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "staged", "staged\n")
	git(t, dir, "add", "staged")
	write(t, dir, "untracked", "untracked\n")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.DiscardChanges(); err != nil {
		t.Fatal(err)
	}
	if s := status(t, r); len(s.Staged)+len(s.Unstaged) != 0 {
		t.Errorf("left %+v", s)
	}
}

// A name is made fit for a branch: what git refuses is corrected or taken out.
func TestSanitiseBranchName(t *testing.T) {
	for name, want := range map[string]string{
		"topic":                 "topic",
		"  my new   branch  ":   "my-new-branch",
		"fix\tthe\nbug":         "fix-the-bug",
		"what? why: *this* ~1^": "what-why-this-1",
		"a[b]\\c":               "ab]c",
		"one..two...three":      "one.two.three",
		"x@{1}":                 "x@1}",
		"/feature//login/":      "feature/login",
		".hidden/.dot.":         "hidden/dot",
		"name.lock":             "name",
		"part.lock/other":       "part/other",
		"--option":              "option",
		"- leading":             "leading",
		"\x01bell\x7f":          "bell",
		"café/ünïcode":          "café/ünïcode",
		"":                      "",
		"~^:?*":                 "",
		"@":                     "",
		" @ ":                   "",
	} {
		if got := SanitiseBranchName(name); got != want {
			t.Errorf("%q gave %q, not %q", name, got, want)
		}
	}
}

// Whatever a name is made into, git takes it as a branch's, unless it is
// nothing.
func TestSanitisedNamesAreValid(t *testing.T) {
	r := newRepo(t)
	for _, name := range []string{
		"a b", "a..b", "a/./b", "a/../b", "a.lock/b.lock", "..", "...lock", "a@{b", "@{", "/", "//", "-", "-a-",
		"a~b^c:d?e*f[g\\h", "tab\tnew\nline", ". .", "x/.lock", "a/.b/c.", "ends.", "dot.lock.lock", "@@{{",
	} {
		got := SanitiseBranchName(name)
		if got != "" && !r.ValidBranchName(got) {
			t.Errorf("%q gave %q, which git refuses", name, got)
		}
	}
}

// The history is the log with, while anything is uncommitted, the commit
// standing for it first, on top of HEAD; more says whether the limit was reached.
func TestHistory(t *testing.T) {
	r := newRepo(t)
	commits, more, err := r.History(10)
	if err != nil || more || len(commits) != 1 || commits[0].Subject != "first" {
		t.Fatalf("clean: %+v, %v, %v", commits, more, err)
	}
	head := commits[0].Hash
	write(t, r.Root, "loose", "new\n")
	write(t, r.Root, "file", "two\n")
	git(t, r.Root, "commit", "-a", "-m", "second")
	commits, more, err = r.History(1)
	if err != nil || !more || len(commits) != 2 {
		t.Fatalf("changed: %+v, %v, %v", commits, more, err)
	}
	if c := commits[0]; c.Hash != Uncommitted || len(c.Parents) != 1 || c.Parents[0] != commits[1].Hash || commits[1].Hash == head {
		t.Errorf("uncommitted %+v on %+v", c, commits[1])
	}

	// A branch with no commits has only what is uncommitted, on nothing.
	git(t, r.Root, "switch", "--orphan", "empty")
	commits, more, err = r.History(10)
	if err != nil || more || len(commits) != 1 || commits[0].Hash != Uncommitted || len(commits[0].Parents) != 0 {
		t.Errorf("unborn: %+v, %v, %v", commits, more, err)
	}

	// A folder that is no longer a repository can't be read.
	if err := os.RemoveAll(filepath.Join(r.Root, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.History(10); err == nil {
		t.Error("no error without a repository")
	}
}

// Anything staged, unstaged or untracked is a change.
func TestHasChanges(t *testing.T) {
	r := newRepo(t)
	if status(t, r).HasChanges() {
		t.Error("a clean tree has changes")
	}
	write(t, r.Root, "new", "new\n")
	if !status(t, r).HasChanges() {
		t.Error("an untracked file is not a change")
	}
	git(t, r.Root, "add", "new")
	if s := status(t, r); !s.HasChanges() || len(s.Unstaged) != 0 {
		t.Errorf("a staged file is not a change: %+v", s)
	}
}

// What git writes to its errors with an empty answer, as a hint, is not taken
// for the answer: reading past the history's end gives no commits.
func TestReadIgnoresHints(t *testing.T) {
	r := newRepo(t)
	write(t, filepath.Join(r.Root, ".git", "info"), "grafts", "")
	commits, err := r.Log(10, 5)
	if err != nil || len(commits) != 0 {
		t.Errorf("past the end gave %v, %v", commits, err)
	}
	if _, err := r.read("log", "-1", "--format=%H"); err != nil {
		t.Fatal(err)
	}
	// What changes something still reports what git said there.
	if out, err := r.run("log", "--skip=5", "--format=%H"); err != nil || !strings.Contains(out, "hint:") {
		t.Errorf("run gave %q, %v", out, err)
	}
}

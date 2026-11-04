// Package git drives the git command line for one repository. It knows nothing
// of the UI, and its methods may be called from any goroutine.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// MaxDiff is the most of a diff that is read, in bytes. The rest of a larger
// one is left out, as no one reads that much and the widget showing it would
// slow to a crawl.
const MaxDiff = 4 << 20

// Codes of a Change, besides git's own M, A, D, R, C and T.
const (
	Untracked = '?'
	Conflict  = 'U'
)

// Change is one changed file, on one side of the index.
type Change struct {
	Path string
	// OldPath is where a renamed or copied file came from.
	OldPath string
	// Code is what happened to the file.
	Code byte
}

// paths returns the paths that a pathspec for the change has to name.
func (c Change) paths() []string {
	if c.OldPath != "" {
		return []string{c.Path, c.OldPath}
	}
	return []string{c.Path}
}

// Status is the state of the work tree and index.
type Status struct {
	// Branch is the checked out branch, empty when HEAD is detached.
	Branch string
	// Upstream is the branch's upstream, if it has one, and Ahead and Behind
	// are how many commits it differs from it by.
	Upstream      string
	Ahead, Behind int
	// Merging says whether a merge is waiting to be committed.
	Merging bool
	// Rebasing says whether a rebase is in progress, and Rebase how far it has
	// got.
	Rebasing bool
	Rebase   RebaseState
	Unstaged []Change
	Staged   []Change
}

// Repo is a git repository.
type Repo struct {
	// Root is the top of the work tree.
	Root string
}

// Open finds the repository that holds dir.
func Open(dir string) (*Repo, error) {
	out, err := (&Repo{Root: dir}).run("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	return &Repo{Root: strings.TrimRight(out, "\n")}, nil
}

// command returns a git command to run at the top of the work tree. Paths are
// always taken literally, git never waits for an editor or a password typed in
// a terminal, and with no terminal of its own in a new session, neither does
// ssh, which would otherwise ask on the terminal the program was started from
// and hang.
func (r *Repo) command(args ...string) *exec.Cmd {
	return r.commandPaths(true, args...)
}

// commandPaths is command, with paths taken literally only if literal says so.
func (r *Repo) commandPaths(literal bool, args ...string) *exec.Cmd {
	options := []string{"-c", "core.quotePath=false"}
	if literal {
		options = append([]string{"--literal-pathspecs"}, options...)
	}
	cmd := exec.Command("git", append(options, args...)...)
	cmd.Dir = r.Root
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_OPTIONAL_LOCKS=0",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

// run runs git and returns what it wrote to its output.
func (r *Repo) run(args ...string) (string, error) {
	return r.runInput("", args...)
}

// runInput runs git with the input and returns what it wrote to its output.
// The error of a failure holds everything git said.
func (r *Repo) runInput(input string, args ...string) (string, error) {
	return r.runCommand(r.command(args...), input)
}

// runCommand runs a git command made by command, as runInput does.
func (r *Repo) runCommand(cmd *exec.Cmd, input string) (string, error) {
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), failure(err, stdout.String(), stderr.String())
	}
	if stdout.Len() == 0 {
		// Some commands, push and fetch among them, report on their errors.
		return stderr.String(), nil
	}
	return stdout.String(), nil
}

// failure returns the error of a command that failed.
func failure(err error, stdout, stderr string) error {
	said := strings.TrimSpace(strings.TrimSpace(stdout) + "\n" + strings.TrimSpace(stderr))
	if said == "" {
		return err
	}
	return errors.New(said)
}

// gitPath returns the absolute path of a file in the repository's git folder.
func (r *Repo) gitPath(name string) (string, error) {
	out, err := r.run("rev-parse", "--path-format=absolute", "--git-path", name)
	return strings.TrimRight(out, "\n"), err
}

// hasCommits says whether HEAD is a commit yet, which on a new branch it is not.
func (r *Repo) hasCommits() bool {
	_, err := r.run("rev-parse", "-q", "--verify", "HEAD")
	return err == nil
}

// Status reads the state of the work tree and index.
func (r *Repo) Status() (Status, error) {
	// The status, whether a merge is going on and a rebase's state are read at
	// once.
	var (
		out           string
		err, mergeErr error
		rebase        RebaseState
		rebasing      bool
	)
	together(
		func() { out, err = r.run("status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all") },
		// The command fails, as it should, when there is no such ref.
		func() { _, mergeErr = r.run("rev-parse", "-q", "--verify", "MERGE_HEAD") },
		func() { rebase, rebasing = r.rebaseState() },
	)
	if err != nil {
		return Status{}, err
	}
	s, err := parseStatus(out)
	if err != nil {
		return Status{}, err
	}
	s.Merging = mergeErr == nil
	s.Rebase, s.Rebasing = rebase, rebasing
	return s, nil
}

// parseStatus reads the output of git status --porcelain=v2 --branch -z.
func parseStatus(out string) (Status, error) {
	var s Status
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue
		}
		var path, oldPath string
		var x, y byte
		switch f[0] {
		case '#':
			s.readHeader(f)
			continue
		case '!':
			continue
		case '?':
			s.Unstaged = append(s.Unstaged, Change{Path: strings.TrimPrefix(f, "? "), Code: Untracked})
			continue
		case '1':
			// 1 XY sub mH mI mW hH hI path
			p := strings.SplitN(f, " ", 9)
			if len(p) != 9 {
				return s, fmt.Errorf("unexpected status line: %q", f)
			}
			path, x, y = p[8], p[1][0], p[1][1]
		case '2':
			// 2 XY sub mH mI mW hH hI Xscore path, then the old path.
			p := strings.SplitN(f, " ", 10)
			if len(p) != 10 || i+1 >= len(fields) {
				return s, fmt.Errorf("unexpected status line: %q", f)
			}
			i++
			path, oldPath, x, y = p[9], fields[i], p[1][0], p[1][1]
		case 'u':
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			p := strings.SplitN(f, " ", 11)
			if len(p) != 11 {
				return s, fmt.Errorf("unexpected status line: %q", f)
			}
			s.Unstaged = append(s.Unstaged, Change{Path: p[10], Code: Conflict})
			continue
		default:
			return s, fmt.Errorf("unexpected status line: %q", f)
		}
		if x != '.' {
			s.Staged = append(s.Staged, sideOf(path, oldPath, x))
		}
		if y != '.' {
			s.Unstaged = append(s.Unstaged, sideOf(path, oldPath, y))
		}
	}
	return s, nil
}

// sideOf returns a line of the status as a change on one side of the index. The
// line's old path only belongs to the side that renamed or copied the file: after
// a rename is staged, the old path is in neither the index nor the work tree, and
// naming it to git on the unstaged side is an error.
func sideOf(path, oldPath string, code byte) Change {
	c := Change{Path: path, Code: code}
	if code == 'R' || code == 'C' {
		c.OldPath = oldPath
	}
	return c
}

// readHeader takes in a "# branch.…" line of the status.
func (s *Status) readHeader(line string) {
	p := strings.Fields(line)
	if len(p) < 3 {
		return
	}
	switch p[1] {
	case "branch.head":
		if p[2] != "(detached)" {
			s.Branch = p[2]
		}
	case "branch.upstream":
		s.Upstream = p[2]
	case "branch.ab":
		if len(p) == 4 {
			s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(p[2], "+"))
			s.Behind, _ = strconv.Atoi(strings.TrimPrefix(p[3], "-"))
		}
	}
}

// patchOptions make a patch look the same whatever the user's configuration, which
// the parsing of it relies on.
var patchOptions = []string{"--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/"}

// Diff returns the diff of a change, from the staged or unstaged side. Anything
// over MaxDiff is cut off, at the end of a line, and a note put in its place.
func (r *Repo) Diff(c Change, staged bool) (string, error) {
	args := append([]string{"diff"}, patchOptions...)
	switch {
	case c.Code == Untracked:
		// Compared with nothing, all of it is new.
		args = append(args, "--no-index", "--", os.DevNull, c.Path)
	case staged:
		args = append(append(args, "--cached", "--"), c.paths()...)
	default:
		args = append(append(args, "--"), c.paths()...)
	}
	return r.capped(args, c.Code == Untracked)
}

// capped runs git and returns what it wrote, cut off as Diff says. differs says
// that an exit code of 1 only means that files differ.
func (r *Repo) capped(args []string, differs bool) (string, error) {
	cmd := r.command(args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(pipe, MaxDiff+1))
	cut := len(data) > MaxDiff
	if cut {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	switch {
	case cut:
		data = data[:bytes.LastIndexByte(data[:MaxDiff], '\n')+1]
		return string(data) + cutNote(), nil
	case readErr != nil:
		return "", readErr
	case waitErr != nil && !(differs && exitCode(waitErr) == 1):
		// With --no-index, diff exits 1 to say that the files differ.
		return "", failure(waitErr, "", stderr.String())
	}
	return string(data), nil
}

// cutNote returns what is said in place of what is cut off a diff.
func cutNote() string {
	return fmt.Sprintf("[The rest is not shown: this diff is over %d MiB.]\n", MaxDiff>>20)
}

// exitCode returns the exit code of a command's error, or -1.
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

// Commit is one commit in the history.
type Commit struct {
	Hash, Short string
	// Parents are the hashes of the commits it follows.
	Parents []string
	Author  string
	Date    time.Time
	// Refs names the branches and tags at the commit, as git writes them.
	Refs    string
	Subject string
	// Message is the whole message, which only RebaseCommits reads.
	Message string
}

// Uncommitted is the hash of the commit standing for the changes not yet
// committed, which no real commit's can be.
const Uncommitted = "uncommitted"

// UncommittedCommit returns the commit standing for the changes not yet
// committed, on top of a commit, if there is one yet.
func UncommittedCommit(parent string) Commit {
	c := Commit{Hash: Uncommitted, Subject: "Uncommitted changes"}
	if parent != "" {
		c.Parents = []string{parent}
	}
	return c
}

// Log returns up to limit commits reachable from HEAD, after skipping some,
// newest first, and every commit before its parents. A branch with no commits yet has none.
func (r *Repo) Log(limit, skip int) ([]Commit, error) {
	if !r.hasCommits() {
		return nil, nil
	}
	out, err := r.run("log", "--topo-order", "--no-color", "--no-show-signature", "--max-count="+strconv.Itoa(limit), "--skip="+strconv.Itoa(skip),
		"--format=%H%x00%h%x00%P%x00%an%x00%at%x00%D%x00%s", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	return parseLog(out)
}

// parseLog reads the output of git log in the format Log asks for.
func parseLog(out string) ([]Commit, error) {
	var commits []Commit
	for line := range strings.Lines(out) {
		f := strings.Split(strings.TrimRight(line, "\n"), "\x00")
		if len(f) != 7 {
			return nil, fmt.Errorf("unexpected log line: %q", line)
		}
		seconds, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected log line: %q", line)
		}
		commits = append(commits, Commit{
			Hash: f[0], Short: f[1], Parents: strings.Fields(f[2]), Author: f[3],
			Date: time.Unix(seconds, 0), Refs: f[5], Subject: f[6],
		})
	}
	return commits, nil
}

// compared returns the arguments of git diff-tree that compare a commit with its
// first parent, or with nothing if it has none. A merge is shown as what it
// brought into its branch.
func compared(c Commit) []string {
	if len(c.Parents) == 0 {
		return []string{"--root", c.Hash}
	}
	return []string{c.Parents[0], c.Hash}
}

// CommitDetail returns the files a commit changed, in the order of its diff, and
// the text of its header, message and diff, cut off as Diff says.
func (r *Repo) CommitDetail(c Commit) ([]Change, string, error) {
	if c.Hash == Uncommitted {
		return r.uncommittedDetail()
	}
	// The files, the header and the diff are read at once.
	var out, header, diff string
	var outErr, headerErr, diffErr error
	together(
		func() {
			out, outErr = r.run(append([]string{"diff-tree", "-r", "-z", "-M", "--no-commit-id", "--name-status"}, compared(c)...)...)
		},
		func() {
			header, headerErr = r.run("log", "-1", "--no-color", "--no-decorate", "--no-show-signature", "--date=iso", "--format=fuller", c.Hash, "--")
		},
		func() {
			diff, diffErr = r.capped(slices.Concat([]string{"diff-tree", "-r", "-p", "-M", "--no-commit-id"}, patchOptions, compared(c)), false)
		},
	)
	if err := firstError(outErr, headerErr, diffErr); err != nil {
		return nil, "", err
	}
	changes, err := parseNameStatus(out)
	if err != nil {
		return nil, "", err
	}
	return changes, strings.TrimRight(header, "\n") + "\n\n" + diff, nil
}

// firstError returns the first of the errors that is not nil, as the reads made
// at once would have failed with had they been made one after another.
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// uncommittedDetail returns the files changed since HEAD, staged or not, then
// those untracked, and their diffs, cut off as Diff says. A branch with no commits
// has no HEAD to compare with, so shows what is staged and what is untracked.
func (r *Repo) uncommittedDetail() ([]Change, string, error) {
	base := "HEAD"
	if !r.hasCommits() {
		base = "--cached"
	}
	// The files, their diff and the status, for the untracked files, are read
	// at once.
	var out, text string
	var s Status
	var outErr, textErr, statusErr error
	together(
		func() { out, outErr = r.run("diff", "-z", "-M", "--name-status", base, "--") },
		func() {
			text, textErr = r.capped(slices.Concat([]string{"diff", "-M"}, patchOptions, []string{base, "--"}), false)
		},
		func() { s, statusErr = r.Status() },
	)
	if err := firstError(outErr, textErr, statusErr); err != nil {
		return nil, "", err
	}
	changes, err := parseNameStatus(out)
	if err != nil {
		return nil, "", err
	}
	untracked := slices.DeleteFunc(slices.Clone(s.Unstaged), func(c Change) bool { return c.Code != Untracked })
	changes = append(changes, untracked...)
	text, err = r.untrackedDiffs(text, untracked)
	if err != nil {
		return nil, "", err
	}
	if len(text) > MaxDiff {
		text = text[:strings.LastIndexByte(text[:MaxDiff], '\n')+1] + cutNote()
	}
	return changes, text, nil
}

// untrackedDiffs returns the text with the diffs of the untracked files after
// it, in their order, until it is cut off, as nothing goes after a diff that has
// been. They are read readers at a time, at once, and only so many more as could
// be needed are read past where it is cut off.
func (r *Repo) untrackedDiffs(text string, untracked []Change) (string, error) {
	cut := func() bool { return len(text) >= MaxDiff || strings.HasSuffix(text, cutNote()) }
	for start := 0; start < len(untracked) && !cut(); start += readers {
		batch := untracked[start:min(start+readers, len(untracked))]
		diffs := make([]string, len(batch))
		errs := make([]error, len(batch))
		each(len(batch), func(i int) { diffs[i], errs[i] = r.Diff(batch[i], false) })
		for i := range batch {
			if cut() {
				break
			}
			if errs[i] != nil {
				return "", errs[i]
			}
			text += diffs[i]
		}
	}
	return text, nil
}

// parseNameStatus reads the output of git diff-tree -z --name-status.
func parseNameStatus(out string) ([]Change, error) {
	changes := []Change{}
	fields := strings.Split(out, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		status := fields[i]
		if status == "" {
			return nil, fmt.Errorf("unexpected diff-tree output: %q", out)
		}
		c := Change{Code: status[0], Path: fields[i+1]}
		// A rename or copy has the old path, then the new one.
		if c.Code == 'R' || c.Code == 'C' {
			i++
			if i+1 >= len(fields) || fields[i+1] == "" {
				return nil, fmt.Errorf("unexpected diff-tree output: %q", out)
			}
			c.OldPath, c.Path = c.Path, fields[i+1]
		}
		changes = append(changes, c)
	}
	return changes, nil
}

// pathList returns the paths of changes in the form --pathspec-file-nul reads,
// which, unlike arguments, has no limit on how many there can be.
func pathList(changes []Change) string {
	var b strings.Builder
	for _, c := range changes {
		for _, p := range c.paths() {
			b.WriteString(p)
			b.WriteByte(0)
		}
	}
	return b.String()
}

// Stage adds the changes to the index.
func (r *Repo) Stage(changes []Change) error {
	if len(changes) == 0 {
		return nil
	}
	_, err := r.runInput(pathList(changes), "add", "-A", "--pathspec-from-file=-", "--pathspec-file-nul")
	return err
}

// Unstage takes the changes out of the index. It always names paths: a reset
// without any would also abandon a merge in progress.
func (r *Repo) Unstage(changes []Change) error {
	if len(changes) == 0 {
		return nil
	}
	_, err := r.runInput(pathList(changes), "reset", "-q", "--pathspec-from-file=-", "--pathspec-file-nul")
	return err
}

// Commit commits the index with the message.
func (r *Repo) Commit(message string) (string, error) {
	return r.runInput(message, "commit", "-F", "-")
}

// MergeMessage returns the message git has prepared for the merge in progress,
// without its comments.
func (r *Repo) MergeMessage() string {
	path, err := r.gitPath("MERGE_MSG")
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	lines = slices.DeleteFunc(lines, func(l string) bool { return strings.HasPrefix(l, "#") })
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Branches returns the names of the local and of the remote tracking branches.
func (r *Repo) Branches() (local, remote []string, err error) {
	out, err := r.run("for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, nil, err
	}
	for ref := range strings.Lines(out) {
		ref = strings.TrimRight(ref, "\n")
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			local = append(local, name)
		} else if name, ok := strings.CutPrefix(ref, "refs/remotes/"); ok && !strings.HasSuffix(name, "/HEAD") {
			remote = append(remote, name)
		}
	}
	return local, remote, nil
}

// SanitiseBranchName returns the name made fit for a branch, as far as it can
// be, by git's rules for the names of refs: runs of white space become a single
// hyphen; control characters and ~ ^ : ? * [ \ go; ".." becomes "." and "@{"
// becomes "@"; each part between slashes loses the dots it starts with and the
// dots and ".lock" it ends with, and empty parts go, so that no slash is doubled
// or at either end; hyphens at the start go, as git would take the name for an
// option; and "@" alone goes, as git takes it for HEAD. What is left may still
// not do, as "HEAD", which ValidBranchName tells.
func SanitiseBranchName(name string) string {
	name = strings.Join(strings.Fields(name), "-")
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune("~^:?*[\\", r) {
			return -1
		}
		return r
	}, name)
	for strings.Contains(name, "..") {
		name = strings.ReplaceAll(name, "..", ".")
	}
	for strings.Contains(name, "@{") {
		name = strings.ReplaceAll(name, "@{", "@")
	}
	var parts []string
	for part := range strings.SplitSeq(name, "/") {
		for {
			trimmed := strings.TrimSuffix(strings.Trim(part, "."), ".lock")
			if trimmed == part {
				break
			}
			part = trimmed
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	name = strings.TrimLeft(strings.Join(parts, "/"), "-")
	if name == "@" {
		return ""
	}
	return name
}

// ValidBranchName says whether a new branch could have the name.
func (r *Repo) ValidBranchName(name string) bool {
	_, err := r.run("check-ref-format", "--branch", name)
	return err == nil
}

// CreateBranch makes a branch at the start point, or at HEAD if that is empty,
// and checks it out if asked to.
func (r *Repo) CreateBranch(name, start string, checkout bool) (string, error) {
	args := []string{"branch", "--", name}
	if checkout {
		args = []string{"switch", "-c", name}
	}
	if start != "" {
		args = append(args, start)
	}
	return r.run(args...)
}

// Checkout switches to a local branch.
func (r *Repo) Checkout(name string) (string, error) {
	return r.run("switch", name)
}

// CheckoutRemote makes a local branch that tracks the remote one, and switches
// to it.
func (r *Repo) CheckoutRemote(name string) (string, error) {
	return r.run("switch", "--track", name)
}

// RenameBranch renames a local branch.
func (r *Repo) RenameBranch(from, to string) (string, error) {
	return r.run("branch", "-m", "--", from, to)
}

// DeleteBranch deletes a local branch. Without force, git refuses one that has
// not been merged.
func (r *Repo) DeleteBranch(name string, force bool) (string, error) {
	flag := "-d"
	if force {
		flag = "-D"
	}
	return r.run("branch", flag, "--", name)
}

// Merge merges the branch into the current one.
func (r *Repo) Merge(name string) (string, error) {
	return r.run("merge", name)
}

// AbortMerge abandons the merge in progress.
func (r *Repo) AbortMerge() (string, error) {
	return r.run("merge", "--abort")
}

// DiscardChanges puts the work tree and index back as the current branch's
// last commit has them, as if it had just been checked out: every change to a
// tracked file is undone, staged or not, untracked files and folders are
// deleted, and a merge in progress is abandoned. Ignored files are kept. A
// branch with no commits yet is left with nothing at all.
func (r *Repo) DiscardChanges() (string, error) {
	reset := []string{"reset", "--hard", "-q", "HEAD"}
	if !r.hasCommits() {
		reset = []string{"read-tree", "--empty"}
	}
	if out, err := r.run(reset...); err != nil {
		return out, err
	}
	return r.run("clean", "-f", "-d", "-q")
}

// Remotes returns the names of the remotes.
func (r *Repo) Remotes() ([]string, error) {
	out, err := r.run("remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// RemoteURL returns the URL of a remote.
func (r *Repo) RemoteURL(name string) (string, error) {
	out, err := r.run("remote", "get-url", "--", name)
	return strings.TrimSpace(out), err
}

// AddRemote adds a remote.
func (r *Repo) AddRemote(name, url string) (string, error) {
	return r.run("remote", "add", "--", name, url)
}

// RenameRemote renames a remote.
func (r *Repo) RenameRemote(from, to string) (string, error) {
	return r.run("remote", "rename", "--", from, to)
}

// RemoveRemote removes a remote.
func (r *Repo) RemoveRemote(name string) (string, error) {
	return r.run("remote", "remove", "--", name)
}

// SetRemoteURL changes the URL of a remote.
func (r *Repo) SetRemoteURL(name, url string) (string, error) {
	return r.run("remote", "set-url", "--", name, url)
}

// Stash is a stash: its ref, as stash@{0}, and what git says of it, as "On
// main: its message".
type Stash struct {
	Ref, Subject string
}

// ErrStashConflicts is in the error of a stash applied with conflicts: what
// did not clash is applied, the files that did are left unmerged with markers,
// and the stash is kept, even by a pop.
var ErrStashConflicts = errors.New("the stash was applied with conflicts")

// ErrStashIndex is in the error of a stash whose staged changes could not be
// put back as they were: nothing was changed, and it could be applied with
// everything unstaged instead.
var ErrStashIndex = errors.New("the stash's staged changes could not be put back")

// runStash runs a stash command. It names no paths, so they are not taken
// literally: with --literal-pathspecs, git stash push --include-untracked saves
// the untracked files but leaves them, as it cleans them with a pathspec of its
// own (":/") that would then be taken as a file's name.
func (r *Repo) runStash(args ...string) (string, error) {
	return r.runCommand(r.commandPaths(false, append([]string{"stash"}, args...)...), "")
}

// StashChanges stashes everything uncommitted, staged or not, and untracked
// files too if asked, with the message, or git's own if it is empty.
func (r *Repo) StashChanges(message string, untracked bool) (string, error) {
	args := []string{"push"}
	if untracked {
		args = append(args, "--include-untracked")
	}
	if message != "" {
		args = append(args, "--message", message)
	}
	return r.runStash(args...)
}

// Stashes returns the stashes, newest first.
func (r *Repo) Stashes() ([]Stash, error) {
	out, err := r.runStash("list", "-z", "--format=%gd%x1f%gs")
	if err != nil {
		return nil, err
	}
	var stashes []Stash
	for entry := range strings.SplitSeq(out, "\x00") {
		if ref, subject, ok := strings.Cut(entry, "\x1f"); ok {
			stashes = append(stashes, Stash{Ref: ref, Subject: subject})
		}
	}
	return stashes, nil
}

// ApplyStash puts the stash's changes back, dropping it once they are if pop
// says so, and with what was staged staged again if index does. A failure
// holds ErrStashConflicts or ErrStashIndex where it is one of those.
func (r *Repo) ApplyStash(ref string, pop, index bool) (string, error) {
	args := []string{"apply"}
	if pop {
		args[0] = "pop"
	}
	if index {
		args = append(args, "--index")
	}
	out, err := r.runStash(append(args, ref)...)
	switch {
	case err == nil:
	case strings.Contains(err.Error(), "CONFLICT ("):
		err = fmt.Errorf("%w: %w", ErrStashConflicts, err)
	case strings.Contains(strings.ToLower(err.Error()), "conflicts in index"):
		err = fmt.Errorf("%w: %w", ErrStashIndex, err)
	}
	return out, err
}

// DropStash deletes the stash.
func (r *Repo) DropStash(ref string) (string, error) {
	return r.runStash("drop", ref)
}

// Fetch fetches from every remote.
func (r *Repo) Fetch() (string, error) {
	return r.run("fetch", "--all")
}

// Pull pulls into the current branch, as the repository is configured to.
func (r *Repo) Pull() (string, error) {
	return r.run("pull")
}

// Push pushes the current branch. With a remote named, it is pushed there and
// made the branch's upstream; otherwise it goes to its upstream. Forcing uses
// --force-with-lease, which refuses to overwrite commits on the remote that
// have not been fetched, so that no one else's work is lost unseen.
func (r *Repo) Push(remote string, force bool) (string, error) {
	args := []string{"push"}
	if force {
		args = append(args, "--force-with-lease")
	}
	if remote != "" {
		args = append(args, "-u", remote, "HEAD")
	}
	return r.run(args...)
}

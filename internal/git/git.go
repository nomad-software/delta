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
	Merging  bool
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
	cmd := exec.Command("git", append([]string{"--literal-pathspecs"}, args...)...)
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
	cmd := r.command(args...)
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

// Status reads the state of the work tree and index.
func (r *Repo) Status() (Status, error) {
	out, err := r.run("status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	s, err := parseStatus(out)
	if err != nil {
		return Status{}, err
	}
	// The command fails, as it should, when there is no such ref.
	_, err = r.run("rev-parse", "-q", "--verify", "MERGE_HEAD")
	s.Merging = err == nil
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
		var c Change
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
			c.Path, x, y = p[8], p[1][0], p[1][1]
		case '2':
			// 2 XY sub mH mI mW hH hI Xscore path, then the old path.
			p := strings.SplitN(f, " ", 10)
			if len(p) != 10 || i+1 >= len(fields) {
				return s, fmt.Errorf("unexpected status line: %q", f)
			}
			i++
			c.Path, c.OldPath, x, y = p[9], fields[i], p[1][0], p[1][1]
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
			c.Code = x
			s.Staged = append(s.Staged, c)
		}
		if y != '.' {
			c.Code = y
			s.Unstaged = append(s.Unstaged, c)
		}
	}
	return s, nil
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

// Diff returns the diff of a change, from the staged or unstaged side. Anything
// over MaxDiff is cut off, at the end of a line, and a note put in its place.
func (r *Repo) Diff(c Change, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff"}
	switch {
	case c.Code == Untracked:
		// Compared with nothing, all of it is new.
		args = append(args, "--no-index", "--", os.DevNull, c.Path)
	case staged:
		args = append(append(args, "--cached", "--"), c.paths()...)
	default:
		args = append(append(args, "--"), c.paths()...)
	}
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
		return string(data) + fmt.Sprintf("[The rest is not shown: this diff is over %d MiB.]\n", MaxDiff>>20), nil
	case readErr != nil:
		return "", readErr
	case waitErr != nil && !(c.Code == Untracked && exitCode(waitErr) == 1):
		// Without --index, diff exits 1 to say that the files differ.
		return "", failure(waitErr, "", stderr.String())
	}
	return string(data), nil
}

// exitCode returns the exit code of a command's error, or -1.
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
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
	out, err := r.run("rev-parse", "--path-format=absolute", "--git-path", "MERGE_MSG")
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(strings.TrimRight(out, "\n"))
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
	return r.run("merge", "--no-edit", name)
}

// AbortMerge abandons the merge in progress.
func (r *Repo) AbortMerge() (string, error) {
	return r.run("merge", "--abort")
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

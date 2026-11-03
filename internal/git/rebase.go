package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Action is what an interactive rebase does with a commit, as its plan, which
// git calls the todo list, names it.
type Action string

// The actions of an interactive rebase.
const (
	// Pick keeps the commit as it is.
	Pick Action = "pick"
	// Reword keeps the commit with a new message.
	Reword Action = "reword"
	// Edit stops after the commit, so that it can be changed.
	Edit Action = "edit"
	// Squash folds the commit into the one before, keeping both messages.
	Squash Action = "squash"
	// Fixup folds the commit into the one before, keeping only that one's
	// message.
	Fixup Action = "fixup"
	// Drop leaves the commit out.
	Drop Action = "drop"
)

// Actions are the actions in the order they are offered: the most used first.
var Actions = []Action{Pick, Fixup, Reword, Edit, Squash, Drop}

// Step is a line of the plan of an interactive rebase: a commit and what to do
// with it. Message is the new message of a commit reworded.
type Step struct {
	Commit  Commit
	Action  Action
	Message string
}

// RebaseCommits returns the commits that an interactive rebase from the commit
// would rewrite, oldest first: the commit and those after it up to HEAD, each
// with its whole message. base is the commit they go on top of, or empty when
// the first of them has no parent. Merges can't be rewritten this way, as git
// would flatten them, so a range holding one is refused.
func (r *Repo) RebaseCommits(from string) (commits []Commit, base string, err error) {
	if _, err := r.run("merge-base", "--is-ancestor", from, "HEAD"); err != nil {
		return nil, "", errors.New("the commit is not in the current branch")
	}
	out, err := r.run("rev-list", "--parents", "-n", "1", from)
	if err != nil {
		return nil, "", err
	}
	parents := strings.Fields(out)[1:]
	if len(parents) > 1 {
		return nil, "", errors.New("the commit is a merge, which an interactive rebase can't keep")
	}
	revisions := "HEAD"
	if len(parents) == 1 {
		base = parents[0]
		revisions = base + "..HEAD"
	}
	out, err = r.run("log", "--reverse", "-z", "--no-color", "--no-show-signature",
		"--format=%H%x00%h%x00%P%x00%an%x00%at%x00%s%x00%B", revisions, "--")
	if err != nil {
		return nil, "", err
	}
	commits, err = parseRebaseLog(out)
	if err != nil {
		return nil, "", err
	}
	for _, c := range commits {
		if len(c.Parents) > 1 {
			return nil, "", fmt.Errorf("the merge %s comes after the commit, which an interactive rebase can't keep", c.Short)
		}
	}
	return commits, base, nil
}

// rebaseFields is how many fields each commit has in the log RebaseCommits reads.
const rebaseFields = 7

// parseRebaseLog reads the log in the format RebaseCommits asks for, in which
// every field ends in a NUL, as -z ends each commit with one too.
func parseRebaseLog(out string) ([]Commit, error) {
	fields := strings.Split(out, "\x00")
	// What follows the last NUL is nothing.
	fields = fields[:len(fields)-1]
	if len(fields)%rebaseFields != 0 {
		return nil, fmt.Errorf("unexpected log: %q", out)
	}
	var commits []Commit
	for i := 0; i < len(fields); i += rebaseFields {
		f := fields[i : i+rebaseFields]
		// Each commit after the first starts after the NUL ending the one
		// before, and git puts a newline there.
		f[0] = strings.TrimPrefix(f[0], "\n")
		seconds, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected log: %q", out)
		}
		commits = append(commits, Commit{
			Hash: f[0], Short: f[1], Parents: strings.Fields(f[2]), Author: f[3],
			Date: time.Unix(seconds, 0), Subject: f[5], Message: strings.TrimSpace(f[6]),
		})
	}
	return commits, nil
}

// todo returns the plan of a rebase in the form git reads. A commit reworded is
// picked and then amended with its new message by a command of the plan's own,
// as git would otherwise wait for an editor to give it one. The message is
// written a line at a time, each line quoted for the shell.
func todo(steps []Step) string {
	var b strings.Builder
	for _, s := range steps {
		action := s.Action
		if action == Reword {
			action = Pick
		}
		fmt.Fprintf(&b, "%s %s %s\n", action, s.Commit.Hash, s.Commit.Subject)
		if s.Action == Reword {
			b.WriteString("exec printf '%s\\n'")
			for line := range strings.SplitSeq(strings.TrimSpace(s.Message), "\n") {
				b.WriteString(" " + shellQuote(strings.TrimRight(line, "\r")))
			}
			b.WriteString(" | git commit --amend --allow-empty --quiet --file=-\n")
		}
	}
	return b.String()
}

// shellQuote returns the text quoted for the shell, which takes everything
// between single quotes as it is.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// todoVariable names the file holding the plan of a rebase, for the command that
// stands in for the editor of it.
const todoVariable = "DELTA_REBASE_TODO"

// Rebase rewrites the commits on top of base, or from the root if base is empty,
// following the steps. Git is given the plan through its editor for the plan,
// which is a command that copies it into place. It stops, saying so, at a
// commit to edit or one that does not apply cleanly, when Rebasing says that the
// rebase is still going.
func (r *Repo) Rebase(base string, steps []Step) (string, error) {
	file, err := os.CreateTemp("", "delta-rebase-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(todo(steps))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	// The plan replaces the one git writes, so that nothing is squashed or any
	// other branch moved unasked, whatever the configuration.
	args := []string{"rebase", "--interactive", "--no-autosquash"}
	if base == "" {
		args = append(args, "--root")
	} else {
		args = append(args, base)
	}
	cmd := r.command(args...)
	cmd.Env = append(cmd.Env, todoVariable+"="+file.Name(), `GIT_SEQUENCE_EDITOR=cp "$`+todoVariable+`"`)
	return r.runCommand(cmd, "")
}

// RebaseState is how far a rebase in progress has got.
type RebaseState struct {
	// Step is the step it is at, of Steps.
	Step, Steps int
	// Amending says that it stopped to edit a commit, rather than at one that
	// did not apply.
	Amending bool
}

// rebaseState reads the state of a rebase in progress, and says whether there is
// one, from the files git keeps it in.
func (r *Repo) rebaseState() (RebaseState, bool) {
	for _, dir := range []struct{ name, step, steps string }{
		{"rebase-merge", "msgnum", "end"},
		{"rebase-apply", "next", "last"},
	} {
		path, err := r.gitPath(dir.name)
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			continue
		}
		number := func(name string) int {
			data, _ := os.ReadFile(filepath.Join(path, name))
			n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
			return n
		}
		_, err = os.Stat(filepath.Join(path, "amend"))
		return RebaseState{Step: number(dir.step), Steps: number(dir.steps), Amending: err == nil}, true
	}
	return RebaseState{}, false
}

// Rebasing says whether a rebase is in progress.
func (r *Repo) Rebasing() bool {
	_, ok := r.rebaseState()
	return ok
}

// ContinueRebase carries on with the rebase in progress. Where it stopped to edit
// a commit, whatever is staged is first added to that commit, as git would
// otherwise refuse to go on.
func (r *Repo) ContinueRebase() (string, error) {
	if state, ok := r.rebaseState(); ok && state.Amending {
		// The command exits 1 to say that something is staged.
		if exitCode(r.command("diff", "--cached", "--quiet").Run()) == 1 {
			if out, err := r.run("commit", "--amend", "--no-edit", "--allow-empty"); err != nil {
				return out, err
			}
		}
	}
	return r.run("rebase", "--continue")
}

// SkipRebase leaves out the commit the rebase stopped at, and carries on.
func (r *Repo) SkipRebase() (string, error) {
	return r.run("rebase", "--skip")
}

// AbortRebase abandons the rebase in progress, putting the branch back as it was.
func (r *Repo) AbortRebase() (string, error) {
	return r.run("rebase", "--abort")
}

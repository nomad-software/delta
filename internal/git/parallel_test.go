package git

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// Every function given to together runs at once: none can finish until all
// have started.
func TestTogetherRunsAtOnce(t *testing.T) {
	const n = 5
	var started sync.WaitGroup
	started.Add(n)
	ran := make([]bool, n)
	fns := make([]func(), n)
	for i := range fns {
		fns[i] = func() {
			started.Done()
			started.Wait()
			ran[i] = true
		}
	}
	done := make(chan struct{})
	go func() {
		together(fns...)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the functions did not run at once")
	}
	if slices.Contains(ran, false) {
		t.Errorf("ran %v", ran)
	}
}

// A snapshot holds what reading each part one after another gives, during a
// merge with stashes.
func TestSnapshot(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "stashed\n")
	if _, err := r.StashChanges("kept", false); err != nil {
		t.Fatal(err)
	}
	git(t, r.Root, "switch", "-c", "topic")
	write(t, r.Root, "file", "theirs\n")
	git(t, r.Root, "commit", "-am", "theirs")
	git(t, r.Root, "switch", "main")
	write(t, r.Root, "file", "ours\n")
	git(t, r.Root, "commit", "-am", "ours")
	if _, err := r.Merge("topic"); err == nil {
		t.Fatal("the merge did not conflict")
	}

	snap := r.Snapshot()
	if snap.Err != nil || snap.BranchErr != nil || snap.StashErr != nil {
		t.Fatal(snap.Err, snap.BranchErr, snap.StashErr)
	}
	s := status(t, r)
	local, _, _ := r.Branches()
	tip := r.ReadTip()
	if !slices.Equal(snap.Status.Unstaged, s.Unstaged) || !slices.Equal(snap.Status.Staged, s.Staged) ||
		snap.Status.Branch != s.Branch || !snap.Status.Merging {
		t.Errorf("status %+v, not %+v", snap.Status, s)
	}
	if !slices.Equal(snap.Local, local) || snap.Stashes != 1 {
		t.Errorf("branches %q, %d stashes", snap.Local, snap.Stashes)
	}
	if snap.QuickFix != tip.QuickFix(true) || snap.CanUncommit != (tip.UncommitRefused() == "") {
		t.Errorf("quick fix %+v, uncommit %v", snap.QuickFix, snap.CanUncommit)
	}
	if snap.MergeMessage == "" || snap.MergeMessage != r.MergeMessage() {
		t.Errorf("merge message %q", snap.MergeMessage)
	}

	// Without a merge there is no message, whatever git left behind.
	if _, err := r.AbortMerge(); err != nil {
		t.Fatal(err)
	}
	if snap := r.Snapshot(); snap.Err != nil || snap.Status.Merging || snap.MergeMessage != "" {
		t.Errorf("after the merge: %+v", snap)
	}
}

// What can't be read of the status leaves the rest of a snapshot unused.
func TestSnapshotOutsideARepository(t *testing.T) {
	isolate(t)
	r := &Repo{Root: t.TempDir()}
	if snap := r.Snapshot(); snap.Err == nil || snap.Local != nil || snap.Stashes != 0 || snap.QuickFix.Head != "" {
		t.Errorf("snapshot %+v", snap)
	}
}

// Reads of one repository from many goroutines at once all give the same, for
// the race detector to look over.
func TestReadsAtOnce(t *testing.T) {
	r := newRepo(t)
	write(t, r.Root, "file", "changed\n")
	write(t, r.Root, "new", "new\n")
	commits, err := r.Log(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := r.Snapshot()
	uncommitted := UncommittedCommit(commits[0].Hash)
	wantFiles, _ := r.CommitFiles(uncommitted)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			snap := r.Snapshot()
			if snap.Err != nil || !slices.Equal(snap.Status.Unstaged, want.Status.Unstaged) || snap.QuickFix != want.QuickFix {
				t.Errorf("snapshot %+v", snap)
			}
			if files, err := r.CommitFiles(uncommitted); err != nil || !slices.Equal(files, wantFiles) {
				t.Errorf("files %+v, %v", files, err)
			}
			if _, err := r.CommitFiles(commits[0]); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

package git

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

// each calls every number once, more than one at a time, and never more than
// readers at a time.
func TestEachLimitsHowManyAtOnce(t *testing.T) {
	old := readers
	readers = 3
	t.Cleanup(func() { readers = old })
	const n = 40
	var running, most atomic.Int32
	calls := make([]int32, n)
	each(n, func(i int) {
		now := running.Add(1)
		for {
			m := most.Load()
			if now <= m || most.CompareAndSwap(m, now) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&calls[i], 1)
		running.Add(-1)
	})
	for i, c := range calls {
		if c != 1 {
			t.Errorf("%d called %d times", i, c)
		}
	}
	if m := most.Load(); m < 2 || m > 3 {
		t.Errorf("at most %d at once", m)
	}
}

// The diffs of many untracked files, read several at a time, are in the order
// of the files, each once.
func TestUntrackedDiffsInOrder(t *testing.T) {
	r := newRepo(t)
	var names []string
	for i := range readers*2 + 3 {
		name := fmt.Sprintf("new%03d", i)
		names = append(names, name)
		write(t, r.Root, name, "text of "+name+"\n")
	}
	changes, text, err := r.CommitDetail(UncommittedCommit(""))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range changes {
		got = append(got, c.Path)
	}
	if !slices.Equal(got, names) {
		t.Errorf("files %q", got)
	}
	at := -1
	for _, name := range names {
		next := strings.Index(text, "+text of "+name+"\n")
		if next <= at || strings.Count(text, "+text of "+name+"\n") != 1 {
			t.Fatalf("%s is not once, after the one before it", name)
		}
		at = next
	}
}

// Untracked files after one whose diff is cut off are listed, but their diffs
// are not shown, however many are read at once: here the cut is well short of
// MaxDiff, as the file's last line is long, so that only the files after it in
// the same batch would show.
func TestUntrackedDiffsStopAtTheCut(t *testing.T) {
	r := newRepo(t)
	for i := range readers * 3 {
		text := fmt.Sprintf("small %d\n", i)
		if i == readers+1 {
			text = "short\n" + strings.Repeat("x", MaxDiff+1) + "\n"
		}
		write(t, r.Root, fmt.Sprintf("new%03d", i), text)
	}
	changes, text, err := r.CommitDetail(UncommittedCommit(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != readers*3 {
		t.Errorf("%d files", len(changes))
	}
	if !strings.HasSuffix(text, cutNote()) || strings.Count(text, cutNote()) != 1 {
		t.Error("not cut off once, at the end")
	}
	for i := range readers * 3 {
		shown := strings.Contains(text, fmt.Sprintf("+small %d\n", i))
		if shown != (i <= readers) {
			t.Errorf("file %d shown %v", i, shown)
		}
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
	_, wantText, _ := r.CommitDetail(UncommittedCommit(commits[0].Hash))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			snap := r.Snapshot()
			if snap.Err != nil || !slices.Equal(snap.Status.Unstaged, want.Status.Unstaged) || snap.QuickFix != want.QuickFix {
				t.Errorf("snapshot %+v", snap)
			}
			if _, text, err := r.CommitDetail(UncommittedCommit(commits[0].Hash)); err != nil || text != wantText {
				t.Errorf("detail %v", err)
			}
			if _, _, err := r.CommitDetail(commits[0]); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

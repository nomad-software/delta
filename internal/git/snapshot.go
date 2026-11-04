package git

// Snapshot is everything a rescan reads of the repository, read at once.
type Snapshot struct {
	// Status is the status, unless Err says why it could not be read, when
	// nothing else in the snapshot is to be used.
	Status Status
	Err    error
	// Local are the local branches, unless BranchErr says why they could not be
	// read, and Stashes how many stashes there are, unless StashErr does.
	Local     []string
	BranchErr error
	Stashes   int
	StashErr  error
	// QuickFix is what a quick fix would do, and CanUncommit whether the last
	// commit can be taken back.
	QuickFix    QuickFix
	CanUncommit bool
	// MergeMessage is the message git prepared for the merge in progress, if
	// there is one.
	MergeMessage string
}

// Snapshot reads the status, the local branches, the stashes, the last commits
// and any merge's message, each in a goroutine of its own, so that a rescan
// waits on the slowest of them rather than on all of them one after another.
func (r *Repo) Snapshot() Snapshot {
	var (
		snap    Snapshot
		tip     Tip
		message string
		stashes []Stash
	)
	together(
		func() { snap.Status, snap.Err = r.Status() },
		func() { snap.Local, _, snap.BranchErr = r.Branches() },
		func() { stashes, snap.StashErr = r.Stashes() },
		func() { tip = r.ReadTip() },
		// The message is read whether or not there is a merge, which the status
		// says, and kept only if there is.
		func() { message = r.MergeMessage() },
	)
	if snap.Err != nil {
		return Snapshot{Err: snap.Err}
	}
	snap.Stashes = len(stashes)
	s := snap.Status
	snap.QuickFix = tip.QuickFix(len(s.Unstaged)+len(s.Staged) > 0)
	snap.CanUncommit = tip.UncommitRefused() == ""
	if s.Merging {
		snap.MergeMessage = message
	}
	return snap
}

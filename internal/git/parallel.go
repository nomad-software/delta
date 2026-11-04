package git

import "sync"

// together runs the functions at once, each in a goroutine of its own, and
// returns once every one has. Each writes only what is its own, which is read
// once together returns, so that the reads of git that a rescan, the history and
// a commit's files make wait on the slowest rather than on all of them.
func together(fns ...func()) {
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Go(fn)
	}
	wg.Wait()
}

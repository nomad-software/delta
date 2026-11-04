package git

import (
	"runtime"
	"sync"
)

// together runs the functions at once, each in a goroutine of its own, and
// returns once every one has. Each writes only what is its own, which is read
// once together returns, so that the reads of git that a rescan, the history and
// a commit's detail make wait on the slowest rather than on all of them.
func together(fns ...func()) {
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Go(fn)
	}
	wg.Wait()
}

// readers is the most git commands each that runs at once, so that a repository
// with thousands of untracked files does not start thousands of processes.
var readers = max(runtime.NumCPU(), 2)

// each calls fn with every number from 0 up to n, at most readers at once, and
// returns once every call has. Each call writes only what is its own.
func each(n int, fn func(i int)) {
	limit := make(chan struct{}, readers)
	var wg sync.WaitGroup
	for i := range n {
		limit <- struct{}{}
		wg.Go(func() {
			defer func() { <-limit }()
			fn(i)
		})
	}
	wg.Wait()
}

package git

import (
	"errors"
	"fmt"
	"slices"
)

// ErrBranchKept is in the error of a branch finished whose deletion git
// refused, as it has commits not merged: the main branch is checked out and
// pulled, and the branch is still there, to be deleted by force if wanted.
var ErrBranchKept = errors.New("the branch was kept")

// MainBranch returns the main branch among the local ones: main, or else
// master, or empty if there is neither.
func MainBranch(local []string) string {
	for _, name := range []string{"main", "master"} {
		if slices.Contains(local, name) {
			return name
		}
	}
	return ""
}

// FinishBranch finishes with a branch: it checks out the main branch, pulls
// into it, and deletes the branch. It stops at the first step that fails, so
// that the branch is deleted only once the main branch is up to date; a
// refusal to delete it, as it is not merged, holds ErrBranchKept.
func (r *Repo) FinishBranch(branch, main string) (string, error) {
	if _, err := r.Checkout(main); err != nil {
		return "", fmt.Errorf("checking out %s: %w", main, err)
	}
	if _, err := r.Pull(); err != nil {
		return "", fmt.Errorf("pulling into %s, so %s was kept: %w", main, branch, err)
	}
	out, err := r.DeleteBranch(branch, false)
	if err != nil {
		if errors.Is(err, ErrNotMerged) {
			return "", fmt.Errorf("%w: %w", ErrBranchKept, err)
		}
		return "", fmt.Errorf("checked out %s, but %s could not be deleted: %w", main, branch, err)
	}
	return out, nil
}

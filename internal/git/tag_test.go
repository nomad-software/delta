package git

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for tag, want := range map[string]Version{
		"v1.2.3":    {"v", 1, 2, 3},
		"0.10.0":    {"", 0, 10, 0},
		"v10.20.30": {"v", 10, 20, 30},
	} {
		if got, ok := ParseVersion(tag); !ok || got != want || got.String() != tag {
			t.Errorf("%q gave %+v, %v", tag, got, ok)
		}
	}
	for _, tag := range []string{"", "v", "1.2", "1.2.3.4", "v01.2.3", "1.2.3-rc.1", "1.2.3+build", "x1.2.3", "V1.2.3", "1..3", "v1.2.-3", "v+1.2.3"} {
		if v, ok := ParseVersion(tag); ok {
			t.Errorf("%q is %+v", tag, v)
		}
	}
}

// The highest version is by number, not by name, ignoring tags that aren't
// versions, and keeps the prefix of the highest.
func TestHighestVersion(t *testing.T) {
	if _, ok := HighestVersion([]string{"release", "v1.2.3-rc.1"}); ok {
		t.Error("found a version among none")
	}
	v, ok := HighestVersion([]string{"v1.9.0", "v1.10.0", "2.0.0-rc.1", "v1.2.30", "nightly", "1.10.0"})
	if !ok || v.String() != "v1.10.0" {
		t.Errorf("got %s, %v", v, ok)
	}
	if v, _ := HighestVersion([]string{"0.1.0", "0.2.0"}); v.String() != "0.2.0" {
		t.Errorf("got %s", v)
	}
}

// releaseRepo returns a repository on main with an upstream in a bare remote.
func releaseRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	r := newRepo(t)
	bare := t.TempDir()
	git(t, bare, "init", "-q", "--bare", "-b", "main")
	git(t, r.Root, "remote", "add", "origin", bare)
	git(t, r.Root, "push", "-q", "-u", "origin", "main")
	return r, bare
}

// A release is read from the tags here and on the remote both, so that it never
// goes below one pushed by someone else; tagging pushes the branch and the tag
// together.
func TestTagRelease(t *testing.T) {
	r, bare := releaseRepo(t)
	rel, err := r.ReadRelease("main")
	if err != nil || rel.Remote != "origin" || rel.Found || rel.Latest.String() != "v0.0.0" || rel.Next().String() != "v0.0.1" {
		t.Fatalf("%+v, %v", rel, err)
	}

	git(t, r.Root, "tag", "v1.2.3")
	git(t, r.Root, "tag", "not-a-version")
	// Someone else has pushed a higher one, not fetched here.
	other := t.TempDir()
	git(t, other, "clone", "-q", bare, ".")
	git(t, other, "tag", "v1.4.0")
	git(t, other, "push", "-q", "origin", "v1.4.0")

	rel, err = r.ReadRelease("main")
	if err != nil || !rel.Found || rel.Latest.String() != "v1.4.0" || !slices.Equal(rel.Tagged, []string{"v1.2.3"}) {
		t.Fatalf("%+v, %v", rel, err)
	}

	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "fix")
	next := rel.Next()
	if _, err := r.TagRelease("main", next, "The fix", rel.Remote); err != nil {
		t.Fatal(err)
	}
	if got := git(t, bare, "tag", "--list"); got != "v1.4.0\nv1.4.1\n" {
		t.Errorf("the remote has %q", got)
	}
	if git(t, bare, "rev-parse", "main") != git(t, r.Root, "rev-parse", "HEAD") {
		t.Error("the branch was not pushed")
	}
	if got := git(t, r.Root, "cat-file", "-t", "v1.4.1"); got != "tag\n" {
		t.Errorf("the tag is a %q, not annotated", got)
	}
	if got := git(t, r.Root, "tag", "-l", "--format=%(contents:subject)", "v1.4.1"); got != "The fix\n" {
		t.Errorf("the message is %q", got)
	}

	// Never backwards, nor the same again.
	for _, v := range []Version{next, {"v", 1, 3, 0}} {
		if _, err := r.TagRelease("main", v, "", rel.Remote); !errors.Is(err, ErrNotNewer) {
			t.Errorf("%s gave %v", v, err)
		}
	}
}

// A push refused leaves no tag behind, so that the release can be tried again.
func TestTagReleaseRefused(t *testing.T) {
	r, bare := releaseRepo(t)
	other := t.TempDir()
	git(t, other, "clone", "-q", bare, ".")
	git(t, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	git(t, other, "push", "-q", "origin", "main")

	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "mine")
	_, err := r.TagRelease("main", Version{"v", 0, 1, 0}, "", "origin")
	if err == nil || !strings.Contains(err.Error(), "deleted again") {
		t.Fatalf("gave %v", err)
	}
	if got := git(t, r.Root, "tag", "--list"); got != "" {
		t.Errorf("tags left %q", got)
	}
	if got := git(t, bare, "tag", "--list"); got != "" {
		t.Errorf("the remote has %q", got)
	}
}

// A branch with no upstream goes to the only remote; with several, to none.
func TestReleaseRemote(t *testing.T) {
	r := newRepo(t)
	if _, err := r.ReadRelease("main"); !errors.Is(err, ErrNoReleaseRemote) {
		t.Errorf("with no remote gave %v", err)
	}
	bare := t.TempDir()
	git(t, bare, "init", "-q", "--bare", "-b", "main")
	git(t, r.Root, "remote", "add", "only", bare)
	if rel, err := r.ReadRelease("main"); err != nil || rel.Remote != "only" {
		t.Errorf("%+v, %v", rel, err)
	}
	git(t, r.Root, "remote", "add", "second", bare)
	if _, err := r.ReadRelease("main"); !errors.Is(err, ErrNoReleaseRemote) {
		t.Errorf("with two remotes gave %v", err)
	}
	if _, err := r.ReadRelease(""); err == nil {
		t.Error("a detached HEAD was released")
	}
	empty := t.TempDir()
	git(t, empty, "init", "-q", "-b", "main")
	if _, err := (&Repo{Root: empty}).ReadRelease("main"); !errors.Is(err, ErrNoCommits) {
		t.Errorf("with no commits gave %v", err)
	}
}

// A branch pushed to an upstream of another name goes there.
func TestTagReleaseToUpstreamOfAnotherName(t *testing.T) {
	r, bare := releaseRepo(t)
	git(t, r.Root, "switch", "-q", "-c", "local")
	git(t, r.Root, "branch", "-q", "-u", "origin/main")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "more")
	if _, err := r.TagRelease("local", Version{"v", 1, 0, 0}, "", "origin"); err != nil {
		t.Fatal(err)
	}
	if git(t, bare, "rev-parse", "main") != git(t, r.Root, "rev-parse", "HEAD") {
		t.Error("main was not pushed")
	}
	if got := git(t, bare, "branch", "--list"); strings.Contains(got, "local") {
		t.Errorf("the remote has %q", got)
	}
}

package git

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Version is a semantic version, as a tag names it: major, minor and patch, with
// the tag's prefix, "v" or none. Pre-releases and build metadata are not
// versions here.
type Version struct {
	Prefix              string
	Major, Minor, Patch int
}

// ParseVersion reads a tag as a version: v1.2.3 or 1.2.3, each number without
// leading zeros. Anything else is not one.
func ParseVersion(tag string) (Version, bool) {
	v := Version{}
	rest := tag
	if strings.HasPrefix(rest, "v") {
		v.Prefix, rest = "v", rest[1:]
	}
	fields := strings.Split(rest, ".")
	if len(fields) != 3 {
		return Version{}, false
	}
	numbers := make([]int, 3)
	for i, f := range fields {
		if f == "" || (len(f) > 1 && f[0] == '0') || strings.TrimLeft(f, "0123456789") != "" {
			return Version{}, false
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return Version{}, false
		}
		numbers[i] = n
	}
	v.Major, v.Minor, v.Patch = numbers[0], numbers[1], numbers[2]
	return v, true
}

// String returns the version as a tag names it.
func (v Version) String() string {
	return fmt.Sprintf("%s%d.%d.%d", v.Prefix, v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0 or 1 as v is lower than, the same as, or higher than o,
// whatever their prefixes.
func (v Version) Compare(o Version) int {
	for _, d := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// HighestVersion returns the highest version the tags name, and whether any
// names one. Of the same version tagged with and without "v", the one with it
// is kept, so that its prefix is the one carried on.
func HighestVersion(tags []string) (Version, bool) {
	var highest Version
	found := false
	for _, tag := range tags {
		v, ok := ParseVersion(tag)
		if !ok {
			continue
		}
		if c := v.Compare(highest); !found || c > 0 || (c == 0 && v.Prefix == "v") {
			highest, found = v, true
		}
	}
	return highest, found
}

// Release is what tagging a release needs to know: the remote it is pushed to,
// the highest version tagged there or here, which the next must be above, and
// the versions HEAD is tagged with already.
type Release struct {
	Remote string
	Latest Version
	// Found says whether any tag names a version; without one, Latest is
	// v0.0.0.
	Found bool
	// Tagged are the versions HEAD is tagged with, if any.
	Tagged []string
}

// Next returns the next patch release, which a release starts as.
func (r Release) Next() Version {
	v := r.Latest
	v.Patch++
	return v
}

// ErrNoCommits is the error of a release of a branch with no commits.
var ErrNoCommits = errors.New("there are no commits to release")

// ErrNoReleaseRemote is the error of a branch with no remote to push a release
// to: no upstream, and not exactly one remote to choose.
var ErrNoReleaseRemote = errors.New("there is no remote to push the release to")

// ReadRelease reads what tagging a release of the branch needs: the remote it
// goes to, its upstream's or else the only one, and the highest version of the
// tags here and on that remote, so that no release goes backwards from one
// someone else has pushed.
func (r *Repo) ReadRelease(branch string) (Release, error) {
	if !r.hasCommits() {
		return Release{}, ErrNoCommits
	}
	remote, err := r.releaseRemote(branch)
	if err != nil {
		return Release{}, err
	}
	var local []string
	var remoteTags, tagged string
	var localErr, remoteErr, taggedErr error
	together(
		func() { local, localErr = r.localTags() },
		func() { remoteTags, remoteErr = r.run("ls-remote", "--tags", "--refs", "--", remote) },
		func() { tagged, taggedErr = r.run("tag", "--points-at", "HEAD") },
	)
	if err := cmp.Or(localErr, remoteErr, taggedErr); err != nil {
		return Release{}, err
	}
	tags := local
	for line := range strings.Lines(remoteTags) {
		if _, ref, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	rel := Release{Remote: remote}
	rel.Latest, rel.Found = HighestVersion(tags)
	if !rel.Found {
		rel.Latest = Version{Prefix: "v"}
	}
	for _, tag := range strings.Fields(tagged) {
		if _, ok := ParseVersion(tag); ok {
			rel.Tagged = append(rel.Tagged, tag)
		}
	}
	return rel, nil
}

// releaseRemote returns the remote a release of the branch is pushed to: the
// branch's upstream's, or else the only remote there is.
func (r *Repo) releaseRemote(branch string) (string, error) {
	if branch == "" {
		return "", errors.New("HEAD is detached, so there is no branch to release")
	}
	// A remote of "." is the repository itself: an upstream that is a local
	// branch.
	if remote := r.branchConfig(branch, "remote"); remote != "" && remote != "." {
		return remote, nil
	}
	remotes, err := r.Remotes()
	if err != nil {
		return "", err
	}
	if len(remotes) != 1 {
		return "", fmt.Errorf("%w: push the branch first, to give it an upstream", ErrNoReleaseRemote)
	}
	return remotes[0], nil
}

// ErrNotNewer is the error of a release whose version is not above every one
// tagged already.
var ErrNotNewer = errors.New("the version is not newer than the latest")

// TagRelease tags HEAD with the version, annotated with the message, then pushes
// the branch and the tag to the remote at once (--atomic), so that either both
// arrive or neither does. The version must be above every one tagged here. If
// the push fails, the tag is deleted again, leaving things as they were.
func (r *Repo) TagRelease(branch string, v Version, message, remote string) (string, error) {
	local, err := r.localTags()
	if err != nil {
		return "", err
	}
	if latest, ok := HighestVersion(local); ok && v.Compare(latest) <= 0 {
		return "", fmt.Errorf("%w: %s is not above %s", ErrNotNewer, v, latest)
	}
	tag := v.String()
	if strings.TrimSpace(message) == "" {
		message = tag
	}
	if _, err := r.run("tag", "-a", "-m", message, "--", tag); err != nil {
		return "", err
	}
	out, err := r.run("push", "--atomic", "--", remote, r.upstreamRefspec(branch), "refs/tags/"+tag)
	if err != nil {
		if _, delErr := r.run("tag", "-d", "--", tag); delErr != nil {
			return "", fmt.Errorf("%w\n\nThe tag %s could not be deleted again: %w", err, tag, delErr)
		}
		return "", fmt.Errorf("%w\n\nNothing was pushed, and the tag %s was deleted again", err, tag)
	}
	return out, nil
}

// upstreamRefspec returns what pushes the branch to its upstream's branch, or
// to one of its own name if it has none.
func (r *Repo) upstreamRefspec(branch string) string {
	target := r.branchConfig(branch, "merge")
	if target == "" {
		target = "refs/heads/" + branch
	}
	return "refs/heads/" + branch + ":" + target
}

// branchConfig returns the branch's setting of the key, as remote or merge, or
// nothing if it has none.
func (r *Repo) branchConfig(branch, key string) string {
	out, err := r.run("config", "--get", "branch."+branch+"."+key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// localTags returns the names of the tags in the repository.
func (r *Repo) localTags() ([]string, error) {
	out, err := r.run("tag", "--list")
	return strings.Fields(out), err
}

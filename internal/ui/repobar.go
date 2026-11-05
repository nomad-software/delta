package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	qt "github.com/mappu/miqt/qt6"
)

// The bar's dropdowns are as wide as their longest names, but no narrower than
// repoComboLeast and no wider than repoComboMost, when the names are cut short.
// They narrow, to the least, where the toolbar has no room for more, so that
// they leave it for its menu of what does not fit only once they can't.
const (
	repoComboLeast = 225
	repoComboMost  = 300
)

// repoBar is at the toolbar's right, centred on its height: a dropdown of the
// repositories opened before, by the names of their folders, and one of the
// local branches of the one open. Choosing in them opens the repository, or
// checks out the branch. It takes up the toolbar's room beyond its buttons, and
// is pushed to the right by a stretch of its own, so that it can be narrowed
// rather than put in the toolbar's menu.
type repoBar struct {
	widget                    *qt.QWidget
	repos, branches           *qt.QComboBox
	reposLabel, branchesLabel *qt.QLabel
	// paths are the folders of the repositories, in the order listed.
	paths []string
}

func newRepoBar() *repoBar {
	b := &repoBar{widget: qt.NewQWidget2()}
	b.widget.SetSizePolicy2(qt.QSizePolicy__Expanding, qt.QSizePolicy__Preferred)
	b.repos = newBarCombo("No repository")
	b.branches = newBarCombo("No branch")
	b.reposLabel = qt.NewQLabel3("Repository")
	b.reposLabel.SetBuddy(b.repos.QWidget)
	b.branchesLabel = qt.NewQLabel3("Branch")
	b.branchesLabel.SetBuddy(b.branches.QWidget)
	layout := qt.NewQHBoxLayout(b.widget)
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.AddStretch()
	layout.AddWidget3(b.reposLabel.QWidget, 0, qt.AlignVCenter)
	layout.AddWidget3(b.repos.QWidget, 0, qt.AlignVCenter)
	layout.AddSpacing(repoBarGap)
	layout.AddWidget3(b.branchesLabel.QWidget, 0, qt.AlignVCenter)
	layout.AddWidget3(b.branches.QWidget, 0, qt.AlignVCenter)
	layout.AddSpacing(repoBarEnd)
	return b
}

// repoBarGap is the space between the two dropdowns, and repoBarEnd the space
// after the branches' dropdown, before the toolbar's end.
const (
	repoBarGap = 12
	repoBarEnd = 12
)

// newBarCombo returns a dropdown of the bar, saying the placeholder while
// nothing is chosen.
func newBarCombo(placeholder string) *qt.QComboBox {
	c := qt.NewQComboBox2()
	c.SetSizeAdjustPolicy(qt.QComboBox__AdjustToContents)
	c.SetMinimumWidth(repoComboLeast)
	c.SetMaximumWidth(repoComboMost)
	c.SetPlaceholderText(placeholder)
	return c
}

// byFolder returns the paths in order of the names of their last folders,
// ignoring case, and of the whole paths where those are the same.
func byFolder(paths []string) []string {
	paths = slices.Clone(paths)
	slices.SortStableFunc(paths, func(a, b string) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(filepath.Base(a)), strings.ToLower(filepath.Base(b))),
			strings.Compare(a, b),
		)
	})
	return paths
}

// setRepositories lists the repositories by the names of their folders, with
// the whole path as each one's tooltip, choosing the one open, if any.
func (b *repoBar) setRepositories(paths []string, current string) {
	b.paths = byFolder(paths)
	b.repos.Clear()
	for i, path := range b.paths {
		b.repos.AddItem(filepath.Base(path))
		tip := qt.NewQVariant11(path)
		b.repos.SetItemData2(i, tip, int(qt.ToolTipRole))
		tip.Delete()
	}
	b.repos.SetCurrentIndex(slices.Index(b.paths, current))
}

// setBranches lists the local branches by name, choosing the current one, which
// is listed even if git does not yet list it, as it does not before the first
// commit. Nothing is chosen on a detached HEAD.
func (b *repoBar) setBranches(local []string, current string) {
	if current != "" && !slices.Contains(local, current) {
		local = append(slices.Clone(local), current)
	}
	local = byName(local)
	b.branches.Clear()
	b.branches.AddItems(local)
	b.branches.SetCurrentIndex(slices.Index(local, current))
}

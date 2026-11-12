package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"
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
	widget                    *qtwidgets.QWidget
	repos, branches           *qtwidgets.QComboBox
	reposLabel, branchesLabel *qtwidgets.QLabel
	// paths are the folders of the repositories, in the order listed.
	paths []string
}

func newRepoBar() *repoBar {
	b := &repoBar{widget: qtwidgets.NewQWidget(nil)}
	setSizePolicy(b.widget, qtwidgets.QSizePolicyExpanding, qtwidgets.QSizePolicyPreferred)
	b.repos = newBarCombo("No repository")
	b.branches = newBarCombo("No branch")
	b.reposLabel = qtwidgets.NewQLabelWithText("Repository", nil)
	b.reposLabel.SetBuddy(b.repos)
	b.branchesLabel = qtwidgets.NewQLabelWithText("Branch", nil)
	b.branchesLabel.SetBuddy(b.branches)
	layout := qtwidgets.NewQHBoxLayout(b.widget)
	layout.SetContentsMargins(qtcore.QMargins{})
	layout.AddStretch()
	layout.AddWidgetFull(b.reposLabel, 0, qtcore.AlignVCenter)
	layout.AddWidgetFull(b.repos, 0, qtcore.AlignVCenter)
	layout.AddSpacing(repoBarGap)
	layout.AddWidgetFull(b.branchesLabel, 0, qtcore.AlignVCenter)
	layout.AddWidgetFull(b.branches, 0, qtcore.AlignVCenter)
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
func newBarCombo(placeholder string) *qtwidgets.QComboBox {
	c := qtwidgets.NewQComboBox(nil)
	c.SetSizeAdjustPolicy(qtwidgets.QComboBoxAdjustToContents)
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
		b.repos.SetItemDataFull(i, qtcore.NewQVariantOf(path), int(qtcore.ToolTipRole))
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

package ui

import (
	"slices"
	"strings"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/nomad-software/delta/internal/git"
)

// formDialog is a small dialog of labelled fields with OK and Cancel.
type formDialog struct {
	*qt.QDialog
	form *qt.QFormLayout
	ok   *qt.QPushButton
	// fieldsValid are what say whether each field can be used.
	fieldsValid []func() bool
	// firstText is the first field of text, which has the focus when the
	// dialog is shown, unless focus says another does.
	firstText *qt.QLineEdit
	focus     *qt.QWidget
	// finished says that the dialog has been answered and is going, so that
	// what comes back to it from the background later has nowhere to go.
	finished bool
}

func newFormDialog(parent *qt.QWidget, title string) *formDialog {
	d := &formDialog{QDialog: qt.NewQDialog(parent), form: qt.NewQFormLayout2()}
	// Fields are as wide as the dialog, whatever the style would have.
	d.form.SetFieldGrowthPolicy(qt.QFormLayout__AllNonFixedFieldsGrow)
	d.SetWindowTitle(title)
	d.SetMinimumWidth(420)
	box := qt.NewQDialogButtonBox5(qt.QDialogButtonBox__Ok|qt.QDialogButtonBox__Cancel, qt.Horizontal)
	box.OnAccepted(d.Accept)
	box.OnRejected(d.Reject)
	d.ok = box.Button(qt.QDialogButtonBox__Ok)
	layout := qt.NewQVBoxLayout(d.QWidget)
	layout.AddLayout(d.form.QLayout)
	layout.AddWidget(box.QWidget)
	// The focus would otherwise go to OK, made before the fields, as it goes
	// in the order widgets are made.
	d.OnShowEvent(func(super func(*qt.QShowEvent), event *qt.QShowEvent) {
		super(event)
		switch {
		case d.focus != nil:
			d.focus.SetFocus()
		case d.firstText != nil:
			d.firstText.SetFocus()
		}
	})
	return d
}

// run waits for an answer, and says whether it was OK. The dialog's fields are
// still there to read until the event loop next runs.
func (d *formDialog) run() bool {
	accepted := d.Exec() == int(qt.QDialog__Accepted)
	d.finished = true
	d.DeleteLater()
	return accepted
}

// addText adds a field of text, with OK enabled only while valid says that what
// is in every such field can be used. Spaces around the text are ignored.
func (d *formDialog) addText(label, initial string, valid func(string) bool) *qt.QLineEdit {
	edit := qt.NewQLineEdit2()
	edit.SetText(initial)
	d.fieldsValid = append(d.fieldsValid, func() bool { return valid(strings.TrimSpace(edit.Text())) })
	edit.OnTextChanged(func(string) { d.check() })
	d.check()
	d.form.AddRow3(label, edit.QWidget)
	if d.firstText == nil {
		d.firstText = edit
	}
	return edit
}

// anyText accepts any text, for a field that may be left empty.
func anyText(string) bool { return true }

// check enables OK if every field can be used.
func (d *formDialog) check() {
	d.ok.SetEnabled(!slices.ContainsFunc(d.fieldsValid, func(valid func() bool) bool { return !valid() }))
}

// addChoice adds a list to choose from.
func (d *formDialog) addChoice(label string, items []string) *qt.QComboBox {
	combo := qt.NewQComboBox2()
	combo.AddItems(items)
	d.form.AddRow3(label, combo.QWidget)
	return combo
}

// askChoice asks for one of the items.
func askChoice(parent *qt.QWidget, title, label string, items []string) (string, bool) {
	d := newFormDialog(parent, title)
	combo := d.addChoice(label, items)
	if !d.run() {
		return "", false
	}
	return combo.CurrentText(), true
}

// askText asks for some text that valid accepts.
func askText(parent *qt.QWidget, title, label, initial string, valid func(string) bool) (string, bool) {
	d := newFormDialog(parent, title)
	edit := d.addText(label, initial, valid)
	if !d.run() {
		return "", false
	}
	return strings.TrimSpace(edit.Text()), true
}

// askYesNo asks a question, with No the answer if return is just pressed, as
// these questions are about what can't be taken back.
func askYesNo(parent *qt.QWidget, title, text string) bool {
	box := qt.NewQMessageBox6(qt.QMessageBox__Warning, title, text, qt.QMessageBox__Yes|qt.QMessageBox__No, parent)
	defer box.DeleteLater()
	box.SetTextFormat(qt.PlainText)
	box.SetDefaultButtonWithButton(qt.QMessageBox__No)
	return box.Exec() == int(qt.QMessageBox__Yes)
}

// maxErrorLines is how many lines of git's output an error shows: its last, the
// ones that say what went wrong.
const maxErrorLines = 30

// showError tells of a failure, without waiting for it to be dismissed.
func showError(parent *qt.QWidget, title, text string) {
	lines := strings.Split(text, "\n")
	if len(lines) > maxErrorLines {
		lines = append([]string{"…"}, lines[len(lines)-maxErrorLines:]...)
	}
	box := qt.NewQMessageBox6(qt.QMessageBox__Critical, title, strings.Join(lines, "\n"), qt.QMessageBox__Ok, parent)
	box.SetTextFormat(qt.PlainText)
	box.OnFinished(func(int) { box.DeleteLater() })
	box.Open()
}

// newCheckoutDialog returns the dialog asking which branch to check out, its
// list of them, and its checkbox: the list has the local branches, with the
// current one chosen, and the remote ones after them while the checkbox is
// ticked, each in order of name.
// OK is enabled only while a branch other than the current one is chosen, as
// there is nothing to do otherwise. Ticking it the first time fetches, in the
// background, and lists the remote branches that gives, or, if it fails, says
// why and lists those already known.
func newCheckoutDialog(parent *qt.QWidget, current string, local, remote []string, fetch func() ([]string, error)) (*formDialog, *qt.QComboBox, *qt.QCheckBox) {
	local, remote = byName(local), byName(remote)
	d := newFormDialog(parent, "Checkout")
	branch := d.addChoice("Branch", local)
	branch.SetCurrentIndex(max(branch.FindText(current), 0))
	d.focus = branch.QWidget
	withRemote := qt.NewQCheckBox3("Include remote branches")
	d.form.AddRowWithWidget(withRemote.QWidget)
	note := qt.NewQLabel2()
	note.SetTextFormat(qt.PlainText)
	note.SetWordWrap(true)
	note.Hide()
	d.form.AddRowWithWidget(note.QWidget)
	fetching := false
	d.fieldsValid = append(d.fieldsValid, func() bool {
		return !fetching && branch.Count() > 0 && branch.CurrentText() != current
	})
	branch.OnCurrentIndexChanged(func(int) { d.check() })
	fill := func() {
		chosen := branch.CurrentText()
		branch.Clear()
		branch.AddItems(local)
		if withRemote.IsChecked() {
			branch.AddItems(remote)
		}
		// A branch no longer listed gives way to the current one.
		at := branch.FindText(chosen)
		if at < 0 {
			at = branch.FindText(current)
		}
		branch.SetCurrentIndex(max(at, 0))
		d.check()
	}
	fetched := false
	withRemote.OnToggled(func(on bool) {
		if !on || fetched {
			fill()
			return
		}
		fetched, fetching = true, true
		branch.SetEnabled(false)
		withRemote.SetEnabled(false)
		note.SetText("Fetching…")
		note.Show()
		d.check()
		go func() {
			got, err := fetch()
			mainthread.Start(func() {
				if d.finished {
					return
				}
				fetching = false
				branch.SetEnabled(true)
				withRemote.SetEnabled(true)
				switch {
				case err != nil:
					note.SetText("Fetching failed, so these are the remote branches as last fetched: " + firstLine(err.Error()))
				case len(got) == 0:
					note.SetText("There are no remote branches.")
					remote = nil
				default:
					note.Hide()
					remote = byName(got)
				}
				fill()
			})
		}()
	})
	d.check()
	return d, branch, withRemote
}

// byName returns the names in order, ignoring case.
func byName(names []string) []string {
	names = slices.Clone(names)
	slices.SortStableFunc(names, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return names
}

// versionMost is the highest number a part of a version can be set to.
const versionMost = 1_000_000

// newTagDialog returns the dialog asking for a release's version and message,
// and what returns the version set. The version starts as the next patch
// release, and each of its parts can be changed, but never to below what keeps
// it above the latest, so that none goes backwards. It notes a HEAD tagged
// already, and changes not committed, which are not in the release.
func newTagDialog(parent *qt.QWidget, rel git.Release, uncommitted bool) (*formDialog, func() git.Version, *qt.QLineEdit) {
	d := newFormDialog(parent, "Tag Release")
	current := rel.Latest.String()
	if !rel.Found {
		current = "No version tags"
	}
	d.form.AddRow3("Current", qt.NewQLabel3(current).QWidget)
	next := rel.Next()
	// The new version is one row: its prefix, if any, then a spin box for
	// each of major, minor and patch, side by side.
	row := qt.NewQWidget2()
	line := qt.NewQHBoxLayout(row)
	line.SetContentsMargins(0, 0, 0, 0)
	if next.Prefix != "" {
		line.AddWidget(qt.NewQLabel3(next.Prefix).QWidget)
	}
	spin := func(name string, value int) *qt.QSpinBox {
		s := qt.NewQSpinBox2()
		s.SetRange(0, versionMost)
		s.SetValue(value)
		s.SetAccessibleName(name)
		// They share the row's width, growing with the dialog's.
		line.AddWidget2(s.QWidget, 1)
		return s
	}
	major, minor, patch := spin("Major", next.Major), spin("Minor", next.Minor), spin("Patch", next.Patch)
	d.form.AddRow3("New", row)
	d.focus = patch.QWidget
	version := func() git.Version {
		return git.Version{Prefix: next.Prefix, Major: major.Value(), Minor: minor.Value(), Patch: patch.Value()}
	}
	// No spin box goes below what keeps the version above the latest: the
	// major never below its major, the minor not below its minor while the
	// major is the same, and the patch not to its patch while both are. A
	// spin box raised to its minimum changes the next one's in turn.
	latest := rel.Latest
	major.SetMinimum(latest.Major)
	limit := func(int) {
		minor.SetMinimum(0)
		if major.Value() == latest.Major {
			minor.SetMinimum(latest.Minor)
		}
		patch.SetMinimum(0)
		if major.Value() == latest.Major && minor.Value() == latest.Minor {
			patch.SetMinimum(latest.Patch + 1)
		}
		d.check()
	}
	// OK can't be had for one not above the latest all the same.
	d.fieldsValid = append(d.fieldsValid, func() bool { return version().Compare(latest) > 0 })
	for _, s := range []*qt.QSpinBox{major, minor, patch} {
		s.OnValueChanged(limit)
	}
	limit(0)
	message := d.addText("Message", "", anyText)
	message.SetPlaceholderText("The tag's name")
	d.form.AddRow3("Remote", qt.NewQLabel3(rel.Remote).QWidget)
	var notes []string
	if len(rel.Tagged) > 0 {
		notes = append(notes, "This commit is tagged "+strings.Join(rel.Tagged, ", ")+" already.")
	}
	if uncommitted {
		notes = append(notes, "Changes not committed are not in the release.")
	}
	notes = append(notes, "The tag and the branch are pushed together.")
	note := qt.NewQLabel3(strings.Join(notes, "\n"))
	note.SetWordWrap(true)
	d.form.AddRowWithWidget(note.QWidget)
	keepFitted(d.QDialog)
	return d, version, message
}

package ui

import (
	"slices"
	"strings"

	qt "github.com/mappu/miqt/qt6"
)

// formDialog is a small dialog of labelled fields with OK and Cancel.
type formDialog struct {
	*qt.QDialog
	form *qt.QFormLayout
	ok   *qt.QPushButton
	// fieldsValid are what say whether each field can be used.
	fieldsValid []func() bool
}

func newFormDialog(parent *qt.QWidget, title string) *formDialog {
	d := &formDialog{QDialog: qt.NewQDialog(parent), form: qt.NewQFormLayout2()}
	d.SetWindowTitle(title)
	d.SetMinimumWidth(420)
	box := qt.NewQDialogButtonBox5(qt.QDialogButtonBox__Ok|qt.QDialogButtonBox__Cancel, qt.Horizontal)
	box.OnAccepted(d.Accept)
	box.OnRejected(d.Reject)
	d.ok = box.Button(qt.QDialogButtonBox__Ok)
	layout := qt.NewQVBoxLayout(d.QWidget)
	layout.AddLayout(d.form.QLayout)
	layout.AddWidget(box.QWidget)
	return d
}

// run waits for an answer, and says whether it was OK. The dialog's fields are
// still there to read until the event loop next runs.
func (d *formDialog) run() bool {
	accepted := d.Exec() == int(qt.QDialog__Accepted)
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
	return edit
}

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
